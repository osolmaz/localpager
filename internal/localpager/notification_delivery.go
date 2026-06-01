package localpager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
		updated, err := markNotificationSent(ctx, pool.GORM(), notification.ID, externalID)
		if err != nil {
			return sent, err
		}
		if updated {
			sent++
		}
	}
	return sent, nil
}

func markNotificationSent(ctx context.Context, db *gorm.DB, notificationID int64, externalID string) (bool, error) {
	now := time.Now().UTC()
	result := db.WithContext(ctx).Model(&Notification{}).Where("id = ? AND status = ?", notificationID, "sending").Updates(map[string]any{
		"status":              "sent",
		"sent_at":             &now,
		"external_message_id": externalID,
		"last_error":          nil,
		"updated_at":          now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
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
	return claimNotificationRows(ctx, db, limit, now)
}

func claimNotificationRows(ctx context.Context, db *gorm.DB, limit int, now time.Time) ([]Notification, error) {
	var claimed []Notification
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		notifications, err := pendingNotificationRows(tx, limit)
		if err != nil {
			return err
		}
		for _, notification := range notifications {
			updated, err := markNotificationSending(tx, notification.ID, now)
			if err != nil {
				return err
			}
			if !updated {
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

func pendingNotificationRows(tx *gorm.DB, limit int) ([]Notification, error) {
	var notifications []Notification
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("status = ? AND destination_kind = ?", "pending", "discord_channel").
		Where(`EXISTS (
			SELECT 1
			FROM localpager_jobs
			JOIN localpager_items ON localpager_items.id = localpager_jobs.item_id
			WHERE localpager_jobs.id = localpager_notifications.job_id
			  AND (localpager_items.latest_content_hash IS NULL OR localpager_items.latest_content_hash = localpager_jobs.content_hash)
		)`).
		Order("created_at ASC").
		Limit(limit).
		Find(&notifications).Error
	return notifications, err
}

func markNotificationSending(tx *gorm.DB, notificationID int64, now time.Time) (bool, error) {
	result := tx.Model(&Notification{}).Where("id = ? AND status = ?", notificationID, "pending").Updates(map[string]any{
		"status":     "sending",
		"attempts":   gorm.Expr("attempts + 1"),
		"updated_at": now,
	})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func suppressSupersededPendingNotifications(ctx context.Context, db *gorm.DB, now time.Time) error {
	return db.WithContext(ctx).Model(&Notification{}).
		Where("status IN ?", []string{"pending", "sending"}).
		Where(`EXISTS (
			SELECT 1
			FROM localpager_jobs
			JOIN localpager_items ON localpager_items.id = localpager_jobs.item_id
			WHERE localpager_jobs.id = localpager_notifications.job_id
			  AND localpager_items.latest_content_hash IS NOT NULL
			  AND localpager_items.latest_content_hash != localpager_jobs.content_hash
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
