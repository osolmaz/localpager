package notifier

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func RecordWatcherSuccess(ctx context.Context, pool *Pool, source, name, cursor string) error {
	now := time.Now().UTC()
	state := WatcherState{
		Source:        source,
		Name:          name,
		Cursor:        stringPtrOrNil(cursor),
		LastRunAt:     &now,
		LastSuccessAt: &now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return upsertWatcherState(ctx, pool, state, map[string]any{
		"cursor":          state.Cursor,
		"last_run_at":     state.LastRunAt,
		"last_success_at": state.LastSuccessAt,
		"last_error":      nil,
		"updated_at":      now,
	})
}

func RecordWatcherError(ctx context.Context, pool *Pool, source, name string, runErr error) error {
	now := time.Now().UTC()
	state := WatcherState{
		Source:    source,
		Name:      name,
		LastRunAt: &now,
		LastError: stringPtr(runErr.Error()),
		CreatedAt: now,
		UpdatedAt: now,
	}
	return upsertWatcherState(ctx, pool, state, map[string]any{
		"last_run_at": state.LastRunAt,
		"last_error":  state.LastError,
		"updated_at":  now,
	})
}

func upsertWatcherState(ctx context.Context, pool *Pool, state WatcherState, updates map[string]any) error {
	return pool.GORM().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "source"}, {Name: "name"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&state).Error
}

func WatcherCursor(ctx context.Context, db *gorm.DB, source, name string) (string, error) {
	var state WatcherState
	err := db.WithContext(ctx).Where("source = ? AND name = ?", source, name).First(&state).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	if state.Cursor == nil {
		return "", nil
	}
	return *state.Cursor, nil
}
