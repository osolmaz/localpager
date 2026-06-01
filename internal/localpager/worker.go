package localpager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/osolmaz/localpager/internal/timing"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const DefaultClassifierCommand = "./scripts/localpager-classifier"
const fallbackClassifierModel = "gemma-4-e4b-it"
const pendingDiscordBatchSize = 10

var errStaleJobLease = errors.New("stale job lease")

var supportedJobTypes = map[string]bool{
	"classify_github_pr":    true,
	"classify_github_issue": true,
}

type WorkerOptions struct {
	MaxConcurrency           int
	LeaseTTL                 time.Duration
	MaxAttempts              int
	Limit                    int
	Once                     bool
	ClassifierCommand        string
	ClassifierSchema         string
	ClassifierPromptTemplate string
	ClassifierTopicTaxonomy  string
	Model                    string
	DestinationRef           string
	DiscordToken             string
	SendDiscord              bool
	DryRunDiscord            bool
	PollInterval             time.Duration
	NotifyTopicsAny          []string
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

	stats, err := state.snapshot()
	if err != nil {
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

func (state *workerRunState) snapshot() (WorkerStats, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.stats, state.firstError
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
			if err := timing.SleepContext(ctx, opts.PollInterval); err != nil {
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
				FROM localpager_items
				WHERE localpager_items.id = localpager_jobs.item_id
				  AND (localpager_items.latest_content_hash IS NULL OR localpager_items.latest_content_hash = localpager_jobs.content_hash)
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
UPDATE localpager_jobs
SET status = 'skipped',
    last_error = 'superseded_content_hash',
    leased_until = NULL,
    run_after = NULL,
    updated_at = ?
WHERE status = 'pending'
  AND EXISTS (
    SELECT 1
    FROM localpager_items
    WHERE localpager_items.id = localpager_jobs.item_id
      AND localpager_items.latest_content_hash IS NOT NULL
      AND localpager_items.latest_content_hash != localpager_jobs.content_hash
  )`, now).Error
}

func processJob(ctx context.Context, pool *Pool, job ClaimedJob, opts WorkerOptions) WorkerStats {
	var stats WorkerStats
	if !supportedJobTypes[job.JobKind] {
		return failClaimedJob(ctx, pool, job, fmt.Sprintf("unsupported job type %s", job.JobKind), opts.MaxAttempts)
	}

	output, outputJSON, promptPath, sessionPath, err := runClassifier(ctx, job, opts)
	if err != nil {
		return failClaimedJob(ctx, pool, job, err.Error(), opts.MaxAttempts)
	}
	result := classifierResult(job, opts, output, outputJSON, promptPath, sessionPath)
	notificationInserted, finalized, err := persistJobResult(ctx, pool, job, opts, output, &result)
	if err != nil {
		return failPersistedJob(ctx, pool, job, err, opts.MaxAttempts)
	}
	if !finalized {
		return stats
	}
	stats.Sent = sendPendingAfterFinalize(ctx, pool, opts)
	if notificationInserted {
		stats.Notifications++
	}
	stats.Succeeded++
	return stats
}

func failClaimedJob(ctx context.Context, pool *Pool, job ClaimedJob, reason string, maxAttempts int) WorkerStats {
	var stats WorkerStats
	if err := markClaimedJobFailed(ctx, pool.GORM(), job, reason, maxAttempts); err == nil {
		stats.Failed++
	}
	return stats
}

func classifierResult(job ClaimedJob, opts WorkerOptions, output ClassifierOutput, outputJSON, promptPath, sessionPath string) Result {
	topicsJSON, _ := json.Marshal(output.TopicsOfInterest)
	return Result{
		ItemID:      job.ItemID,
		JobID:       job.ID,
		JobKind:     job.JobKind,
		OutputJSON:  outputJSON,
		TopicsJSON:  stringPtrOrNil(string(topicsJSON)),
		SessionPath: stringPtrOrNil(sessionPath),
		PromptPath:  stringPtrOrNil(promptPath),
		Model:       stringPtrOrNil(opts.Model),
		CreatedAt:   time.Now().UTC(),
	}
}

func persistJobResult(ctx context.Context, pool *Pool, job ClaimedJob, opts WorkerOptions, output ClassifierOutput, result *Result) (bool, bool, error) {
	notificationInserted := false
	finalized := false
	err := retrySQLiteBusy(ctx, func() error {
		return pool.GORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			created, err := createResultAndNotification(ctx, tx, job, opts, output, result)
			if err != nil {
				return err
			}
			if result.ID == 0 {
				return nil
			}
			notificationInserted = created
			if err := markClaimedJobSucceeded(ctx, tx, job); err != nil {
				return err
			}
			finalized = true
			return nil
		})
	})
	return notificationInserted, finalized, err
}

func createResultAndNotification(ctx context.Context, tx *gorm.DB, job ClaimedJob, opts WorkerOptions, output ClassifierOutput, result *Result) (bool, error) {
	superseded, err := skipClaimedJobIfSuperseded(ctx, tx, job)
	if err != nil || superseded {
		return false, err
	}
	if err := tx.Create(result).Error; err != nil {
		return false, err
	}
	return insertNotificationIfNeeded(tx, job, opts, output, result.ID)
}

func insertNotificationIfNeeded(tx *gorm.DB, job ClaimedJob, opts WorkerOptions, output ClassifierOutput, resultID int64) (bool, error) {
	if !shouldNotify(output, opts) {
		return false, nil
	}
	notification := buildNotification(job, opts, output, resultID)
	created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&notification)
	if created.Error != nil {
		return false, created.Error
	}
	return created.RowsAffected > 0, nil
}

func buildNotification(job ClaimedJob, opts WorkerOptions, output ClassifierOutput, resultID int64) Notification {
	status, sentAt, suppression := notificationInitialState(job)
	now := time.Now().UTC()
	return Notification{
		ItemID:            job.ItemID,
		ResultID:          resultID,
		JobID:             job.ID,
		NotificationKind:  "github_interest",
		DestinationKind:   "discord_channel",
		DestinationRef:    opts.DestinationRef,
		MessageKey:        fmt.Sprintf("%d:%d:github_interest", job.ItemID, resultID),
		MessageBody:       buildNotificationMessage(job, output),
		Status:            status,
		SuppressionReason: suppression,
		SentAt:            sentAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func notificationInitialState(job ClaimedJob) (string, *time.Time, *string) {
	if job.NotificationSuppressionReason == nil || *job.NotificationSuppressionReason == "" {
		return "pending", nil, nil
	}
	now := time.Now().UTC()
	return "sent", &now, job.NotificationSuppressionReason
}

func markClaimedJobSucceeded(ctx context.Context, tx *gorm.DB, job ClaimedJob) error {
	update := tx.Model(&Job{}).
		Where("id = ? AND status = ? AND leased_until = ?", job.ID, "running", job.LeasedUntil).
		Where(`EXISTS (
			SELECT 1
			FROM localpager_items
			WHERE localpager_items.id = localpager_jobs.item_id
			  AND (localpager_items.latest_content_hash IS NULL OR localpager_items.latest_content_hash = localpager_jobs.content_hash)
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
	return nil
}

func failPersistedJob(ctx context.Context, pool *Pool, job ClaimedJob, err error, maxAttempts int) WorkerStats {
	var stats WorkerStats
	if errors.Is(err, errStaleJobLease) {
		return stats
	}
	_ = markJobFailed(ctx, pool.GORM(), job.ID, err.Error(), maxAttempts)
	stats.Failed++
	return stats
}

func sendPendingAfterFinalize(ctx context.Context, pool *Pool, opts WorkerOptions) int {
	var sent int
	err := retrySQLiteBusy(ctx, func() error {
		var sendErr error
		sent, sendErr = SendPendingDiscord(ctx, pool, opts)
		return sendErr
	})
	if err != nil {
		return 0
	}
	return sent
}

func skipClaimedJobIfSuperseded(ctx context.Context, tx *gorm.DB, job ClaimedJob) (bool, error) {
	now := time.Now().UTC()
	result := tx.WithContext(ctx).Model(&Job{}).
		Where("id = ? AND status = ? AND leased_until = ?", job.ID, "running", job.LeasedUntil).
		Where(`EXISTS (
			SELECT 1
			FROM localpager_items
			WHERE localpager_items.id = localpager_jobs.item_id
			  AND localpager_items.latest_content_hash IS NOT NULL
			  AND localpager_items.latest_content_hash != localpager_jobs.content_hash
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
