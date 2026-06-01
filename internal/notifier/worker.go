package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DefaultClassifierCommand = "localpager-classifier"
const fallbackClassifierModel = "gemma-4-e4b-it"
const pendingDiscordBatchSize = 10

var errStaleJobLease = errors.New("stale job lease")

var supportedJobTypes = map[string]bool{
	"classify_github_pr":    true,
	"classify_github_issue": true,
}

type WorkerOptions struct {
	MaxConcurrency    int
	LeaseTTL          time.Duration
	MaxAttempts       int
	Limit             int
	Once              bool
	ClassifierCommand string
	Model             string
	DestinationRef    string
	DiscordToken      string
	SendDiscord       bool
	DryRunDiscord     bool
	PollInterval      time.Duration
}

type WorkerStats struct {
	Claimed       int
	Succeeded     int
	Failed        int
	Notifications int
	Sent          int
}

type ClaimedJob struct {
	Job
	Item Item
}

type ClassifierOutput struct {
	Caveats          []string `json:"caveats"`
	Confidence       float64  `json:"confidence"`
	Interest         string   `json:"interest"`
	TopicsOfInterest []string `json:"topics_of_interest"`
	Description      string   `json:"description"`
}

func RunWorker(ctx context.Context, pool *Pool, opts WorkerOptions) (WorkerStats, error) {
	if opts.MaxConcurrency <= 0 {
		opts.MaxConcurrency = 2
	}
	if opts.LeaseTTL == 0 {
		opts.LeaseTTL = 30 * time.Minute
	}
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.ClassifierCommand == "" {
		opts.ClassifierCommand = DefaultClassifierCommand
	}
	if opts.Model == "" {
		opts.Model = DefaultClassifierModel()
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 30 * time.Second
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	state := workerRunState{}
	var wg sync.WaitGroup
	for range opts.MaxConcurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorkerSlot(runCtx, cancel, pool, opts, &state)
		}()
	}
	wg.Wait()

	stats := state.statsSnapshot()
	if err := state.errSnapshot(); err != nil {
		return stats, err
	}
	return stats, nil
}

type workerRunState struct {
	mu                    sync.Mutex
	stats                 WorkerStats
	reserved              int
	firstError            error
	lastSupersededSweepAt time.Time
}

func (state *workerRunState) reserveClaim(limit int) bool {
	if limit <= 0 {
		return true
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.stats.Claimed+state.reserved >= limit {
		return false
	}
	state.reserved++
	return true
}

func (state *workerRunState) finishClaim(claimed int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.reserved > 0 {
		state.reserved--
	}
	state.stats.Claimed += claimed
}

func (state *workerRunState) addStats(local WorkerStats) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.stats.Succeeded += local.Succeeded
	state.stats.Failed += local.Failed
	state.stats.Notifications += local.Notifications
	state.stats.Sent += local.Sent
}

func (state *workerRunState) addSent(sent int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.stats.Sent += sent
}

func (state *workerRunState) setError(err error) {
	if err == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.firstError == nil {
		state.firstError = err
	}
}

func (state *workerRunState) reserveSupersededSweep(now time.Time) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.lastSupersededSweepAt.IsZero() && now.Sub(state.lastSupersededSweepAt) < 5*time.Minute {
		return false
	}
	state.lastSupersededSweepAt = now
	return true
}

func (state *workerRunState) statsSnapshot() WorkerStats {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.stats
}

func (state *workerRunState) errSnapshot() error {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.firstError
}

func runWorkerSlot(ctx context.Context, cancel context.CancelFunc, pool *Pool, opts WorkerOptions, state *workerRunState) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if !state.reserveClaim(opts.Limit) {
			return
		}
		sweepSuperseded := state.reserveSupersededSweep(time.Now().UTC())
		var jobs []ClaimedJob
		err := retrySQLiteBusy(ctx, func() error {
			var claimErr error
			jobs, claimErr = claimJobs(ctx, pool.GORM(), 1, opts.LeaseTTL, opts.MaxAttempts, 1, sweepSuperseded)
			return claimErr
		})
		if err != nil {
			state.finishClaim(0)
			state.setError(err)
			cancel()
			return
		}
		state.finishClaim(len(jobs))
		if len(jobs) == 0 {
			var sent int
			err := retrySQLiteBusy(ctx, func() error {
				var sendErr error
				sent, sendErr = SendPendingDiscord(ctx, pool, opts)
				return sendErr
			})
			state.addSent(sent)
			if err != nil {
				state.setError(err)
				cancel()
				return
			}
			if opts.Once || opts.Limit > 0 {
				return
			}
			if err := sleepContext(ctx, opts.PollInterval); err != nil {
				if !errors.Is(err, context.Canceled) {
					state.setError(err)
				}
				return
			}
			continue
		}
		state.addStats(processJob(ctx, pool, jobs[0], opts))
	}
}

func DefaultClassifierModel() string {
	if value := strings.TrimSpace(os.Getenv("LOCALPAGER_CLASSIFIER_MODEL")); value != "" {
		return value
	}
	return fallbackClassifierModel
}

func sleepContext(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func claimJobs(ctx context.Context, db *gorm.DB, max int, leaseTTL time.Duration, maxAttempts int, remaining int, sweepSuperseded bool) ([]ClaimedJob, error) {
	if remaining == 0 {
		return nil, nil
	}
	if remaining > 0 && remaining < max {
		max = remaining
	}
	now := time.Now().UTC()
	if err := db.WithContext(ctx).Model(&Job{}).
		Where("status = ? AND leased_until IS NOT NULL AND leased_until < ? AND attempts >= ?", "running", now, maxAttempts).
		Updates(map[string]any{"status": "dead", "leased_until": nil, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&Job{}).
		Where("status = ? AND leased_until IS NOT NULL AND leased_until < ? AND attempts < ?", "running", now, maxAttempts).
		Updates(map[string]any{"status": "pending", "leased_until": nil, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	if sweepSuperseded {
		if err := skipSupersededPendingJobs(ctx, db, now); err != nil {
			return nil, err
		}
	}

	var claimed []ClaimedJob
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []Job
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND attempts < ? AND (run_after IS NULL OR run_after <= ?)", "pending", maxAttempts, now).
			Where(`EXISTS (
				SELECT 1
				FROM notifier_items
				WHERE notifier_items.id = notifier_jobs.item_id
				  AND (notifier_items.latest_content_hash IS NULL OR notifier_items.latest_content_hash = notifier_jobs.content_hash)
			)`).
			Order("CASE WHEN notification_suppression_reason IS NULL THEN 0 ELSE 1 END, priority ASC, created_at ASC").
			Limit(max).
			Find(&jobs).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(leaseTTL)
		for _, job := range jobs {
			result := tx.Model(&Job{}).Where("id = ? AND status = ? AND attempts < ? AND (run_after IS NULL OR run_after <= ?)", job.ID, "pending", maxAttempts, now).Updates(map[string]any{
				"status":       "running",
				"attempts":     gorm.Expr("attempts + 1"),
				"leased_until": &leaseUntil,
				"run_after":    nil,
				"last_error":   nil,
				"updated_at":   now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			var item Item
			if err := tx.First(&item, job.ItemID).Error; err != nil {
				return err
			}
			job.Attempts++
			job.Status = "running"
			job.LeasedUntil = &leaseUntil
			claimed = append(claimed, ClaimedJob{Job: job, Item: item})
		}
		return nil
	})
	return claimed, err
}

func skipSupersededPendingJobs(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Exec(`
UPDATE notifier_jobs
SET status = 'skipped',
    last_error = 'superseded_content_hash',
    leased_until = NULL,
    run_after = NULL,
    updated_at = ?
WHERE status = 'pending'
  AND EXISTS (
    SELECT 1
    FROM notifier_items
    WHERE notifier_items.id = notifier_jobs.item_id
      AND notifier_items.latest_content_hash IS NOT NULL
      AND notifier_items.latest_content_hash != notifier_jobs.content_hash
  )`, now).Error
}

func processJob(ctx context.Context, pool *Pool, job ClaimedJob, opts WorkerOptions) WorkerStats {
	var stats WorkerStats
	if !supportedJobTypes[job.JobKind] {
		if err := markClaimedJobFailed(ctx, pool.GORM(), job, fmt.Sprintf("unsupported job type %s", job.JobKind), opts.MaxAttempts); err == nil {
			stats.Failed++
		}
		return stats
	}

	output, outputJSON, promptPath, sessionPath, err := runClassifier(ctx, job, opts)
	if err != nil {
		if err := markClaimedJobFailed(ctx, pool.GORM(), job, err.Error(), opts.MaxAttempts); err == nil {
			stats.Failed++
		}
		return stats
	}
	topicsJSON, _ := json.Marshal(output.TopicsOfInterest)
	result := Result{
		ItemID:      job.ItemID,
		JobID:       job.ID,
		JobKind:     job.JobKind,
		OutputJSON:  outputJSON,
		Interest:    stringPtrOrNil(output.Interest),
		TopicsJSON:  stringPtrOrNil(string(topicsJSON)),
		SessionPath: stringPtrOrNil(sessionPath),
		PromptPath:  stringPtrOrNil(promptPath),
		Model:       stringPtrOrNil(opts.Model),
		CreatedAt:   time.Now().UTC(),
	}
	notificationInserted := false
	finalized := false
	err = retrySQLiteBusy(ctx, func() error {
		return pool.GORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			superseded, err := skipClaimedJobIfSuperseded(ctx, tx, job)
			if err != nil {
				return err
			}
			if superseded {
				return nil
			}
			if err := tx.Create(&result).Error; err != nil {
				return err
			}
			if shouldNotify(output) {
				status := "pending"
				var sentAt *time.Time
				var suppression *string
				if job.NotificationSuppressionReason != nil && *job.NotificationSuppressionReason != "" {
					status = "sent"
					now := time.Now().UTC()
					sentAt = &now
					suppression = job.NotificationSuppressionReason
				}
				notification := Notification{
					ItemID:            job.ItemID,
					ResultID:          result.ID,
					JobID:             job.ID,
					NotificationKind:  "github_interest",
					DestinationKind:   "discord_channel",
					DestinationRef:    opts.DestinationRef,
					MessageKey:        fmt.Sprintf("%d:%d:github_interest", job.ItemID, result.ID),
					MessageBody:       buildNotificationMessage(job, output),
					Status:            status,
					SuppressionReason: suppression,
					SentAt:            sentAt,
					CreatedAt:         time.Now().UTC(),
					UpdatedAt:         time.Now().UTC(),
				}
				created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notification)
				if created.Error != nil {
					return created.Error
				}
				notificationInserted = created.RowsAffected > 0
			}
			update := tx.Model(&Job{}).
				Where("id = ? AND status = ? AND leased_until = ?", job.ID, "running", job.LeasedUntil).
				Where(`EXISTS (
				SELECT 1
				FROM notifier_items
				WHERE notifier_items.id = notifier_jobs.item_id
				  AND (notifier_items.latest_content_hash IS NULL OR notifier_items.latest_content_hash = notifier_jobs.content_hash)
			)`).
				Updates(map[string]any{
					"status":       "succeeded",
					"leased_until": nil,
					"updated_at":   time.Now().UTC(),
				})
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected == 0 {
				return errStaleJobLease
			}
			finalized = true
			return nil
		})
	})
	if err != nil {
		if errors.Is(err, errStaleJobLease) {
			return stats
		}
		_ = markJobFailed(ctx, pool.GORM(), job.ID, err.Error(), opts.MaxAttempts)
		stats.Failed++
		return stats
	}
	if !finalized {
		return stats
	}
	var sent int
	err = retrySQLiteBusy(ctx, func() error {
		var sendErr error
		sent, sendErr = SendPendingDiscord(ctx, pool, opts)
		return sendErr
	})
	if err == nil {
		stats.Sent += sent
	}
	if notificationInserted {
		stats.Notifications++
	}
	stats.Succeeded++
	return stats
}

func runClassifier(ctx context.Context, job ClaimedJob, opts WorkerOptions) (ClassifierOutput, string, string, string, error) {
	commandPath, err := ExpandPath(opts.ClassifierCommand)
	if err != nil {
		return ClassifierOutput{}, "", "", "", err
	}
	target := deref(job.Item.SourceURL)
	if target == "" {
		target = deref(job.Item.Ref)
	}
	if target == "" {
		target = job.Item.SourceRef
	}
	args := []string{target}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	jobCtx := ctx
	cancel := func() {}
	if job.LeasedUntil != nil {
		timeout := time.Until(*job.LeasedUntil)
		if timeout <= 0 {
			return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier lease already expired")
		}
		jobCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	cmd := exec.CommandContext(jobCtx, commandPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
			return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier timed out before lease expiry")
		}
		return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier failed: %w stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	outputJSON := strings.TrimSpace(stdout.String())
	var output ClassifierOutput
	if err := json.Unmarshal([]byte(outputJSON), &output); err != nil {
		return ClassifierOutput{}, "", "", "", fmt.Errorf("classifier returned invalid JSON: %w stdout=%s stderr=%s", err, outputJSON, strings.TrimSpace(stderr.String()))
	}
	return output, outputJSON, parsePromptPath(stderr.String()), parseSessionPath(stderr.String()), nil
}

func skipClaimedJobIfSuperseded(ctx context.Context, tx *gorm.DB, job ClaimedJob) (bool, error) {
	now := time.Now().UTC()
	result := tx.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status = ? AND leased_until = ?", job.ID, "running", job.LeasedUntil).
		Where(`EXISTS (
			SELECT 1
			FROM notifier_items
			WHERE notifier_items.id = notifier_jobs.item_id
			  AND notifier_items.latest_content_hash IS NOT NULL
			  AND notifier_items.latest_content_hash != notifier_jobs.content_hash
		)`).
		Updates(map[string]any{
			"status":       "skipped",
			"last_error":   "superseded_content_hash",
			"leased_until": nil,
			"run_after":    nil,
			"updated_at":   now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func shouldNotify(output ClassifierOutput) bool {
	interest := strings.ToLower(strings.TrimSpace(output.Interest))
	switch interest {
	case "", "none", "no", "low", "irrelevant", "i0", "false":
		return false
	default:
		return true
	}
}

func buildNotificationMessage(job ClaimedJob, output ClassifierOutput) string {
	title := deref(job.Item.Title)
	if title == "" {
		title = job.Item.SourceRef
	}
	topics := "none"
	if len(output.TopicsOfInterest) > 0 {
		topics = strings.Join(output.TopicsOfInterest, ", ")
	}
	message := fmt.Sprintf("%s\n%s\nInterest: %s\nTopics: %s\n%s", title, deref(job.Item.SourceURL), output.Interest, topics, output.Description)
	if len(message) > 1900 {
		return message[:1900] + "\n..."
	}
	return message
}

func markJobFailed(ctx context.Context, db *gorm.DB, jobID int64, reason string, maxAttempts int) error {
	var job Job
	if err := db.WithContext(ctx).First(&job, jobID).Error; err != nil {
		return err
	}
	return markJobFailedStatus(ctx, db, jobID, job.Attempts, reason, maxAttempts, nil)
}

func markClaimedJobFailed(ctx context.Context, db *gorm.DB, job ClaimedJob, reason string, maxAttempts int) error {
	return markJobFailedStatus(ctx, db, job.ID, job.Attempts, reason, maxAttempts, job.LeasedUntil)
}

func markJobFailedStatus(ctx context.Context, db *gorm.DB, jobID int64, attempts int, reason string, maxAttempts int, leasedUntil *time.Time) error {
	status := "pending"
	runAfter := (*time.Time)(nil)
	if attempts >= maxAttempts {
		status = "dead"
	} else {
		next := time.Now().UTC().Add(retryDelay(attempts))
		runAfter = &next
	}
	query := db.WithContext(ctx).Model(&Job{}).Where("id = ?", jobID)
	if leasedUntil != nil {
		query = query.Where("status = ? AND leased_until = ?", "running", leasedUntil)
	}
	result := query.Updates(map[string]any{
		"status":       status,
		"leased_until": nil,
		"run_after":    runAfter,
		"last_error":   reason,
		"updated_at":   time.Now().UTC(),
	})
	if result.Error != nil {
		return result.Error
	}
	if leasedUntil != nil && result.RowsAffected == 0 {
		return errStaleJobLease
	}
	return nil
}

func retryDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 6 {
		attempts = 6
	}
	return time.Duration(1<<(attempts-1)) * time.Minute
}

func retrySQLiteBusy(ctx context.Context, fn func() error) error {
	var err error
	delay := 250 * time.Millisecond
	for attempt := 0; attempt < 8; attempt++ {
		err = fn()
		if !isSQLiteBusy(err) {
			return err
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 2*time.Second {
			delay *= 2
		}
	}
	return err
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sql logic error: database is locked")
}

func parsePromptPath(stderr string) string {
	re := regexp.MustCompile(`(?m)^prompt:\s*(.+)$`)
	match := re.FindStringSubmatch(stderr)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func parseSessionPath(stderr string) string {
	re := regexp.MustCompile(`(?m)^session:\s*(.+)$`)
	match := re.FindStringSubmatch(stderr)
	if len(match) == 2 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func SendPendingDiscord(ctx context.Context, pool *Pool, opts WorkerOptions) (int, error) {
	if !opts.SendDiscord {
		return 0, nil
	}
	if opts.DiscordToken == "" && !opts.DryRunDiscord {
		return 0, nil
	}
	totalSent := 0
	for {
		notifications, err := claimPendingNotifications(ctx, pool.GORM(), pendingDiscordBatchSize)
		if err != nil {
			return totalSent, err
		}
		if len(notifications) == 0 {
			return totalSent, nil
		}
		sent, err := sendClaimedDiscordNotifications(ctx, pool, opts, notifications)
		totalSent += sent
		if err != nil {
			return totalSent, err
		}
		if sent == 0 {
			return totalSent, nil
		}
	}
}

func sendClaimedDiscordNotifications(ctx context.Context, pool *Pool, opts WorkerOptions, notifications []Notification) (int, error) {
	sent := 0
	for i, notification := range notifications {
		current, stillSending, err := sendingNotification(ctx, pool.GORM(), notification.ID)
		if err != nil {
			return sent, err
		}
		if !stillSending {
			continue
		}
		notification = current
		externalID := "dry-run"
		if !opts.DryRunDiscord {
			var err error
			externalID, err = sendDiscordMessageFunc(ctx, opts.DiscordToken, notification.DestinationRef, notification.MessageBody)
			if err != nil {
				if isPermanentDiscordSendError(err) {
					if markErr := markNotificationSendFailed(ctx, pool.GORM(), notification.ID, err); markErr != nil {
						return sent, markErr
					}
					continue
				}
				_ = resetUnsentNotifications(ctx, pool.GORM(), notifications[i:], err)
				return sent, err
			}
		}
		now := time.Now().UTC()
		result := pool.GORM().WithContext(ctx).Model(&Notification{}).Where("id = ? AND status = ?", notification.ID, "sending").Updates(map[string]any{
			"status":              "sent",
			"sent_at":             &now,
			"external_message_id": externalID,
			"last_error":          nil,
			"updated_at":          now,
		})
		if result.Error != nil {
			return sent, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		sent++
	}
	return sent, nil
}

func sendingNotification(ctx context.Context, db *gorm.DB, id int64) (Notification, bool, error) {
	var notification Notification
	err := db.WithContext(ctx).Where("id = ? AND status = ?", id, "sending").First(&notification).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Notification{}, false, nil
	}
	if err != nil {
		return Notification{}, false, err
	}
	return notification, true, nil
}

func resetUnsentNotifications(ctx context.Context, db *gorm.DB, notifications []Notification, sendErr error) error {
	if len(notifications) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(notifications))
	for _, notification := range notifications {
		ids = append(ids, notification.ID)
	}
	return db.WithContext(ctx).Model(&Notification{}).Where("id IN ? AND status = ?", ids, "sending").Updates(map[string]any{
		"status":     "pending",
		"last_error": sendErr.Error(),
		"updated_at": time.Now().UTC(),
	}).Error
}

func markNotificationSendFailed(ctx context.Context, db *gorm.DB, notificationID int64, sendErr error) error {
	return db.WithContext(ctx).Model(&Notification{}).Where("id = ? AND status = ?", notificationID, "sending").Updates(map[string]any{
		"status":     "failed",
		"last_error": sendErr.Error(),
		"updated_at": time.Now().UTC(),
	}).Error
}

func claimPendingNotifications(ctx context.Context, db *gorm.DB, limit int) ([]Notification, error) {
	now := time.Now().UTC()
	if err := suppressSupersededPendingNotifications(ctx, db, now); err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&Notification{}).
		Where("status = ? AND updated_at < ?", "sending", now.Add(-10*time.Minute)).
		Updates(map[string]any{"status": "pending", "updated_at": now}).Error; err != nil {
		return nil, err
	}
	var claimed []Notification
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var notifications []Notification
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status = ? AND destination_kind = ?", "pending", "discord_channel").
			Where(`EXISTS (
				SELECT 1
				FROM notifier_jobs
				JOIN notifier_items ON notifier_items.id = notifier_jobs.item_id
				WHERE notifier_jobs.id = notifier_notifications.job_id
				  AND (notifier_items.latest_content_hash IS NULL OR notifier_items.latest_content_hash = notifier_jobs.content_hash)
			)`).
			Order("created_at ASC").
			Limit(limit).
			Find(&notifications).Error; err != nil {
			return err
		}
		for _, notification := range notifications {
			result := tx.Model(&Notification{}).Where("id = ? AND status = ?", notification.ID, "pending").Updates(map[string]any{
				"status":     "sending",
				"attempts":   gorm.Expr("attempts + 1"),
				"updated_at": now,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			notification.Status = "sending"
			notification.Attempts++
			claimed = append(claimed, notification)
		}
		return nil
	})
	return claimed, err
}

func suppressSupersededPendingNotifications(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Model(&Notification{}).
		Where("status IN ?", []string{"pending", "sending"}).
		Where(`EXISTS (
			SELECT 1
			FROM notifier_jobs
			JOIN notifier_items ON notifier_items.id = notifier_jobs.item_id
			WHERE notifier_jobs.id = notifier_notifications.job_id
			  AND notifier_items.latest_content_hash IS NOT NULL
			  AND notifier_items.latest_content_hash != notifier_jobs.content_hash
		)`).
		Updates(map[string]any{
			"status":              "sent",
			"suppression_reason":  "superseded_content_hash",
			"sent_at":             &now,
			"external_message_id": nil,
			"last_error":          nil,
			"updated_at":          now,
		}).Error
}

func RequireTokenFromEnv(name string) (string, error) {
	token := os.Getenv(name)
	if token == "" {
		return "", errors.New("missing Discord token env " + name)
	}
	return token, nil
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
