package localpager

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func insertJob(ctx context.Context, tx *gorm.DB, itemID int64, jobType, processorName, processorVersion, contentHash string, priority int, suppressionReason string) (bool, error) {
	status := "pending"
	if suppressionReason != "" {
		status = "skipped"
	}
	now := time.Now().UTC()
	job := Job{
		ItemID:                        itemID,
		JobKind:                       jobType,
		ProcessorName:                 processorName,
		ProcessorVersion:              processorVersion,
		ContentHash:                   contentHash,
		Priority:                      priority,
		Status:                        status,
		NotificationSuppressionReason: stringPtrOrNil(suppressionReason),
		CreatedAt:                     now,
		UpdatedAt:                     now,
	}
	result := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&job)
	if result.Error != nil || result.RowsAffected > 0 {
		return result.RowsAffected > 0, result.Error
	}
	if suppressionReason != "" {
		return false, nil
	}
	return reactivateSkippedCurrentJob(ctx, tx, itemID, processorName, processorVersion, contentHash, priority)
}

func reactivateSkippedCurrentJob(ctx context.Context, tx *gorm.DB, itemID int64, processorName, processorVersion, contentHash string, priority int) (bool, error) {
	result := tx.WithContext(ctx).Model(&Job{}).
		Where("item_id = ? AND processor_name = ? AND processor_version = ? AND content_hash = ?", itemID, processorName, processorVersion, contentHash).
		Where("status = ? AND last_error IN ?", "skipped", []string{"source_suppressed", "superseded_content_hash"}).
		Updates(map[string]any{
			"status":                          "pending",
			"attempts":                        0,
			"priority":                        priority,
			"notification_suppression_reason": nil,
			"last_error":                      nil,
			"leased_until":                    nil,
			"run_after":                       nil,
			"updated_at":                      time.Now().UTC(),
		})
	return result.RowsAffected > 0, result.Error
}

func skipPendingJobsForItem(ctx context.Context, tx *gorm.DB, itemID int64, reason string) error {
	if err := tx.WithContext(ctx).Model(&Job{}).
		Where("item_id = ? AND status IN ?", itemID, []string{"pending", "running"}).
		Updates(map[string]any{
			"status":       "skipped",
			"last_error":   reason,
			"updated_at":   time.Now().UTC(),
			"leased_until": nil,
			"run_after":    nil,
		}).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	return tx.WithContext(ctx).Model(&Notification{}).
		Where("item_id = ? AND status IN ?", itemID, []string{"pending", "sending"}).
		Updates(map[string]any{
			"status":              "sent",
			"suppression_reason":  reason,
			"sent_at":             &now,
			"external_message_id": nil,
			"last_error":          nil,
			"updated_at":          now,
		}).Error
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
