package localpager

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type RequeueJobsOptions struct {
	Statuses          []string
	LastErrorContains string
	DryRun            bool
}

func RequeueJobs(ctx context.Context, pool *Pool, opts RequeueJobsOptions) (int64, error) {
	if pool == nil || pool.GORM() == nil {
		return 0, fmt.Errorf("database pool is not initialized")
	}
	statuses := normalizeStatuses(opts.Statuses)
	if len(statuses) == 0 {
		statuses = []string{"dead"}
	}
	query := pool.GORM().WithContext(ctx).Model(&Job{}).Where("status IN ?", statuses)
	if strings.TrimSpace(opts.LastErrorContains) != "" {
		query = query.Where("last_error LIKE ?", "%"+opts.LastErrorContains+"%")
	}
	if opts.DryRun {
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return 0, err
		}
		return count, nil
	}
	result := query.Updates(map[string]any{
		"status":       "pending",
		"attempts":     0,
		"leased_until": nil,
		"run_after":    nil,
		"last_error":   nil,
		"updated_at":   time.Now().UTC(),
	})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func normalizeStatuses(statuses []string) []string {
	normalized := make([]string, 0, len(statuses))
	seen := map[string]bool{}
	for _, status := range statuses {
		status = strings.ToLower(strings.TrimSpace(status))
		if status == "" || seen[status] {
			continue
		}
		seen[status] = true
		normalized = append(normalized, status)
	}
	return normalized
}
