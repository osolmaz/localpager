package localpager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IngestItem struct {
	Source      string         `json:"source"`
	Type        string         `json:"type"`
	Ref         string         `json:"ref"`
	URL         string         `json:"url"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	State       string         `json:"state"`
	Author      string         `json:"author"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ContentHash string         `json:"content_hash"`
	Metadata    map[string]any `json:"metadata"`
	Suppressed  bool           `json:"suppressed"`
}

type IngestOptions struct {
	JobType                       string
	ProcessorName                 string
	ProcessorVersion              string
	Priority                      int
	InitialHydration              bool
	NotificationSuppressionReason string
	CutoverAt                     *time.Time
	SuppressionReason             string
}

type IngestResult struct {
	ItemID      int64
	JobInserted bool
	JobSkipped  bool
	JobExisting bool
	Suppressed  bool
}

type Ingestor interface {
	Ingest(ctx context.Context, item IngestItem, opts IngestOptions) (IngestResult, error)
}

type DBIngestor struct {
	pool *Pool
}

func NewIngestor(pool *Pool) DBIngestor {
	return DBIngestor{pool: pool}
}

func (ingestor DBIngestor) Ingest(ctx context.Context, item IngestItem, opts IngestOptions) (IngestResult, error) {
	return Ingest(ctx, ingestor.pool, item, opts)
}

func (ingestor DBIngestor) ActiveJobRefs(ctx context.Context, source, itemType string) (map[string]struct{}, error) {
	if ingestor.pool == nil || ingestor.pool.GORM() == nil {
		return nil, fmt.Errorf("database pool is not initialized")
	}
	var refs []string
	err := ingestor.pool.GORM().WithContext(ctx).
		Model(&Item{}).
		Joins("LEFT JOIN localpager_jobs ON localpager_jobs.item_id = localpager_items.id AND localpager_jobs.status IN ?", []string{"pending", "running"}).
		Joins("LEFT JOIN localpager_notifications ON localpager_notifications.item_id = localpager_items.id AND localpager_notifications.status IN ?", []string{"pending", "sending"}).
		Where("localpager_items.source = ? AND localpager_items.type = ? AND localpager_items.ref IS NOT NULL", source, itemType).
		Where("localpager_jobs.id IS NOT NULL OR localpager_notifications.id IS NOT NULL").
		Group("localpager_items.ref").
		Pluck("localpager_items.ref", &refs).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		result[ref] = struct{}{}
	}
	return result, nil
}

func Ingest(ctx context.Context, pool *Pool, item IngestItem, opts IngestOptions) (IngestResult, error) {
	if pool == nil || pool.GORM() == nil {
		return IngestResult{}, fmt.Errorf("database pool is not initialized")
	}
	normalized, err := normalizeIngestItem(item)
	if err != nil {
		return IngestResult{}, err
	}
	opts = normalizeIngestOptions(opts, normalized)

	var result IngestResult
	err = pool.GORM().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		stored, err := upsertGenericItem(ctx, tx, normalized)
		if err != nil {
			return err
		}
		result.ItemID = stored.ID
		if normalized.Suppressed {
			result.Suppressed = true
			if !storedItemHasContentHash(stored, normalized.ContentHash) {
				return nil
			}
			reason := opts.SuppressionReason
			if reason == "" {
				reason = "source_suppressed"
			}
			return skipPendingJobsForItem(ctx, tx, stored.ID, reason)
		}

		suppressionReason := opts.NotificationSuppressionReason
		if suppressionReason == "" && beforeIngestCutover(normalized, opts.CutoverAt) {
			suppressionReason = "pre_cutover"
		}
		inserted, err := insertJob(ctx, tx, stored.ID, opts.JobType, opts.ProcessorName, opts.ProcessorVersion, normalized.ContentHash, opts.Priority, suppressionReason)
		if err != nil {
			return err
		}
		if inserted {
			result.JobInserted = true
			result.JobSkipped = suppressionReason != ""
		} else {
			result.JobExisting = true
		}
		return nil
	})
	return result, err
}

func normalizeIngestOptions(opts IngestOptions, item IngestItem) IngestOptions {
	if opts.ProcessorName == "" {
		opts.ProcessorName = DefaultProcessorName
	}
	if opts.ProcessorVersion == "" {
		opts.ProcessorVersion = DefaultProcessorVer
	}
	if opts.JobType == "" {
		opts.JobType = "classify_" + item.Type
	}
	if opts.InitialHydration && opts.NotificationSuppressionReason == "" {
		opts.NotificationSuppressionReason = "initial_hydration"
	}
	if opts.Priority == 0 {
		opts.Priority = 100
	}
	return opts
}

func storedItemHasContentHash(stored Item, contentHash string) bool {
	return stored.LatestContentHash != nil && *stored.LatestContentHash == contentHash
}

func normalizeIngestItem(item IngestItem) (IngestItem, error) {
	item.Source = strings.TrimSpace(item.Source)
	item.Type = strings.TrimSpace(item.Type)
	item.Ref = strings.TrimSpace(item.Ref)
	item.URL = strings.TrimSpace(item.URL)
	item.Title = strings.TrimSpace(item.Title)
	item.State = strings.TrimSpace(item.State)
	item.Author = strings.TrimSpace(item.Author)
	item.ContentHash = strings.TrimSpace(item.ContentHash)
	if item.Source == "" {
		return IngestItem{}, fmt.Errorf("source is required")
	}
	if item.Type == "" {
		return IngestItem{}, fmt.Errorf("type is required")
	}
	if item.Ref == "" {
		return IngestItem{}, fmt.Errorf("ref is required")
	}
	updatedAtProvided := !item.UpdatedAt.IsZero()
	if updatedAtProvided {
		item.UpdatedAt = item.UpdatedAt.UTC()
	}
	if item.ContentHash == "" {
		hash, err := hashIngestItem(item)
		if err != nil {
			return IngestItem{}, err
		}
		item.ContentHash = hash
	}
	if !updatedAtProvided {
		item.UpdatedAt = time.Now().UTC()
	}
	return item, nil
}

func hashIngestItem(item IngestItem) (string, error) {
	payload := struct {
		Source    string         `json:"source"`
		Type      string         `json:"type"`
		Ref       string         `json:"ref"`
		URL       string         `json:"url"`
		Title     string         `json:"title"`
		Body      string         `json:"body"`
		State     string         `json:"state"`
		Author    string         `json:"author"`
		UpdatedAt time.Time      `json:"updated_at"`
		Metadata  map[string]any `json:"metadata"`
	}{
		Source:    item.Source,
		Type:      item.Type,
		Ref:       item.Ref,
		URL:       item.URL,
		Title:     item.Title,
		Body:      item.Body,
		State:     item.State,
		Author:    item.Author,
		UpdatedAt: item.UpdatedAt,
		Metadata:  item.Metadata,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal item for content hash: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func upsertGenericItem(ctx context.Context, tx *gorm.DB, item IngestItem) (Item, error) {
	now := time.Now().UTC()
	metadataJSON, err := marshalMetadata(item.Metadata)
	if err != nil {
		return Item{}, err
	}
	stored := Item{
		Source:            stringPtr(item.Source),
		Type:              stringPtr(item.Type),
		Ref:               stringPtr(item.Ref),
		SourceKind:        item.Type,
		SourceRef:         legacySourceRef(item),
		SourceURL:         stringPtrOrNil(item.URL),
		Title:             stringPtrOrNil(item.Title),
		State:             stringPtrOrNil(item.State),
		Author:            stringPtrOrNil(item.Author),
		LatestContentHash: stringPtr(item.ContentHash),
		MetadataJSON:      metadataJSON,
		SourceUpdatedAt:   &item.UpdatedAt,
		FirstSeenAt:       now,
		LastSeenAt:        now,
		UpdatedAt:         now,
	}
	freshSource := "localpager_items.source_updated_at IS NULL OR excluded.source_updated_at >= localpager_items.source_updated_at"
	freshColumn := func(column string) clause.Expr {
		return gorm.Expr("CASE WHEN " + freshSource + " THEN excluded." + column + " ELSE localpager_items." + column + " END")
	}
	err = tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "source"}, {Name: "type"}, {Name: "ref"}},
		DoUpdates: clause.Assignments(map[string]any{
			"source_kind":         stored.SourceKind,
			"source_ref":          stored.SourceRef,
			"source_url":          freshColumn("source_url"),
			"title":               freshColumn("title"),
			"state":               freshColumn("state"),
			"author":              freshColumn("author"),
			"latest_content_hash": freshColumn("latest_content_hash"),
			"metadata_json":       freshColumn("metadata_json"),
			"source_updated_at":   freshColumn("source_updated_at"),
			"last_seen_at":        now,
			"updated_at":          now,
		}),
	}).Create(&stored).Error
	if err != nil {
		return Item{}, err
	}
	err = tx.WithContext(ctx).Where("source = ? AND type = ? AND ref = ?", item.Source, item.Type, item.Ref).First(&stored).Error
	return stored, err
}

func legacySourceRef(item IngestItem) string {
	return item.Source + ":" + item.Ref
}

func marshalMetadata(metadata map[string]any) (*string, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return stringPtr(string(encoded)), nil
}

func beforeIngestCutover(item IngestItem, cutoverAt *time.Time) bool {
	return cutoverAt != nil && item.UpdatedAt.Before(cutoverAt.UTC())
}
