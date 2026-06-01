package gitcrawl

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/localpager"
	"github.com/osolmaz/localpager/internal/sources"
)

const DefaultDBPath = "~/.config/gitcrawl/gitcrawl.db"

type EnqueueOptions struct {
	Repo                          string
	Type                          string
	Limit                         int
	InitialHydration              bool
	ProcessorName                 string
	ProcessorVersion              string
	RecentWindow                  time.Duration
	NotificationSuppressionReason string
	CutoverAt                     *time.Time
}

type EnqueueStats = sources.EnqueueStats

type Thread struct {
	Type             string
	Number           int
	State            string
	Title            string
	Author           sql.NullString
	URL              string
	ContentHash      string
	UpdatedAtGH      sql.NullString
	UpdatedAt        string
	ClosedAtLocal    sql.NullString
	CloseReasonLocal sql.NullString
}

type activeJobRefProvider interface {
	ActiveJobRefs(ctx context.Context, source, itemType string) (map[string]struct{}, error)
}

type activeNumbers struct {
	pullRequests map[int]struct{}
	issues       map[int]struct{}
}

func OpenDB(ctx context.Context, path string) (*sql.DB, error) {
	expanded, err := localpager.ExpandPath(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", expanded)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func Enqueue(ctx context.Context, ingestor localpager.Ingestor, db *sql.DB, opts EnqueueOptions) (EnqueueStats, error) {
	if opts.Repo == "" {
		opts.Repo = localpager.DefaultRepo
	}
	if opts.Type == "" {
		opts.Type = "both"
	}
	if opts.ProcessorName == "" {
		opts.ProcessorName = localpager.DefaultProcessorName
	}
	if opts.ProcessorVersion == "" {
		opts.ProcessorVersion = localpager.DefaultProcessorVer
	}
	if opts.RecentWindow == 0 {
		opts.RecentWindow = 48 * time.Hour
	}
	if opts.InitialHydration && opts.NotificationSuppressionReason == "" {
		opts.NotificationSuppressionReason = "initial_hydration"
	}

	active, err := activeNumbersFor(ctx, ingestor, opts.Repo, opts.Type)
	if err != nil {
		return EnqueueStats{}, err
	}
	threads, err := readThreads(ctx, db, opts.Repo, opts.Type, opts.Limit, active)
	if err != nil {
		return EnqueueStats{}, err
	}
	stats := EnqueueStats{ItemsSeen: len(threads)}
	for _, thread := range threads {
		item := MapThread(opts.Repo, thread)
		result, err := ingestor.Ingest(ctx, item, localpager.IngestOptions{
			JobType:                       jobTypeFor(thread),
			ProcessorName:                 opts.ProcessorName,
			ProcessorVersion:              opts.ProcessorVersion,
			Priority:                      PriorityFor(thread, opts.RecentWindow),
			InitialHydration:              opts.InitialHydration,
			NotificationSuppressionReason: opts.NotificationSuppressionReason,
			CutoverAt:                     opts.CutoverAt,
		})
		if err != nil {
			return stats, err
		}
		stats.ItemsUpserted++
		if result.JobInserted {
			stats.JobsInserted++
		}
		if result.JobSkipped {
			stats.JobsSkipped++
		}
		if result.JobExisting {
			stats.JobsExisting++
		}
	}
	return stats, nil
}

func activeNumbersFor(ctx context.Context, ingestor localpager.Ingestor, repo, itemType string) (activeNumbers, error) {
	provider, ok := ingestor.(activeJobRefProvider)
	if !ok {
		return activeNumbers{}, nil
	}
	result := activeNumbers{}
	if sources.WantsPullRequests(itemType) {
		refs, err := provider.ActiveJobRefs(ctx, "gitcrawl", "github_pr")
		if err != nil {
			return activeNumbers{}, err
		}
		result.pullRequests = numbersFromRefs(repo, refs)
	}
	if sources.WantsIssues(itemType) {
		refs, err := provider.ActiveJobRefs(ctx, "gitcrawl", "github_issue")
		if err != nil {
			return activeNumbers{}, err
		}
		result.issues = numbersFromRefs(repo, refs)
	}
	return result, nil
}

func readThreads(ctx context.Context, db *sql.DB, repo, itemType string, limit int, active activeNumbers) ([]Thread, error) {
	typeSQL := "AND threads.kind IN ('issue', 'pull_request')"
	switch itemType {
	case "pr", "prs", "pull_request", "pull_requests", "github_pr":
		typeSQL = "AND threads.kind = 'pull_request'"
	case "issue", "issues", "github_issue":
		typeSQL = "AND threads.kind = 'issue'"
	case "both", "all":
	default:
		return nil, fmt.Errorf("unknown type %q", itemType)
	}
	limitSQL := ""
	args := []any{repo}
	if limit > 0 {
		limitSQL = "LIMIT ?"
		args = append(args, limit)
	}
	query := fmt.Sprintf(`
	SELECT threads.kind, threads.number, threads.state, threads.title, threads.author_login,
	       threads.html_url, threads.content_hash, threads.updated_at_gh, threads.updated_at,
	       threads.closed_at_local, threads.close_reason_local
	FROM threads
	JOIN repositories ON repositories.id = threads.repo_id
WHERE repositories.full_name = ?
  AND threads.state = 'open'
  %s
ORDER BY COALESCE(threads.updated_at_gh, threads.updated_at) DESC, threads.number DESC
	%s`, typeSQL, limitSQL)
	threads, err := queryThreads(ctx, db, query, args...)
	if err != nil {
		return nil, err
	}

	activeThreads, err := readActiveThreads(ctx, db, repo, active)
	if err != nil {
		return nil, err
	}
	return mergeThreads(threads, activeThreads), nil
}

func readActiveThreads(ctx context.Context, db *sql.DB, repo string, active activeNumbers) ([]Thread, error) {
	conditions := []string{}
	args := []any{repo}
	if len(active.pullRequests) > 0 {
		conditions = append(conditions, "threads.kind = 'pull_request' AND threads.number IN ("+placeholders(len(active.pullRequests))+")")
		for number := range active.pullRequests {
			args = append(args, number)
		}
	}
	if len(active.issues) > 0 {
		conditions = append(conditions, "threads.kind = 'issue' AND threads.number IN ("+placeholders(len(active.issues))+")")
		for number := range active.issues {
			args = append(args, number)
		}
	}
	if len(conditions) == 0 {
		return nil, nil
	}
	query := fmt.Sprintf(`
SELECT threads.kind, threads.number, threads.state, threads.title, threads.author_login,
       threads.html_url, threads.content_hash, threads.updated_at_gh, threads.updated_at,
       threads.closed_at_local, threads.close_reason_local
FROM threads
JOIN repositories ON repositories.id = threads.repo_id
WHERE repositories.full_name = ?
  AND (%s)
ORDER BY COALESCE(threads.updated_at_gh, threads.updated_at) DESC, threads.number DESC`, strings.Join(conditions, " OR "))
	return queryThreads(ctx, db, query, args...)
}

func queryThreads(ctx context.Context, db *sql.DB, query string, args ...any) ([]Thread, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var threads []Thread
	for rows.Next() {
		var thread Thread
		if err := rows.Scan(&thread.Type, &thread.Number, &thread.State, &thread.Title, &thread.Author, &thread.URL, &thread.ContentHash, &thread.UpdatedAtGH, &thread.UpdatedAt, &thread.ClosedAtLocal, &thread.CloseReasonLocal); err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func MapThread(repo string, thread Thread) localpager.IngestItem {
	itemType := "github_issue"
	if thread.Type == "pull_request" {
		itemType = "github_pr"
	}
	return localpager.IngestItem{
		Source:      "gitcrawl",
		Type:        itemType,
		Ref:         fmt.Sprintf("%s#%d", repo, thread.Number),
		URL:         thread.URL,
		Title:       thread.Title,
		State:       thread.State,
		Author:      sqlNullString(thread.Author),
		UpdatedAt:   updatedAt(thread),
		ContentHash: thread.ContentHash,
		Suppressed:  LocallySuppressed(thread),
		Metadata: map[string]any{
			"repo":   repo,
			"number": thread.Number,
		},
	}
}

func LocallySuppressed(thread Thread) bool {
	return thread.State != "open" ||
		(thread.ClosedAtLocal.Valid && thread.ClosedAtLocal.String != "") ||
		(thread.CloseReasonLocal.Valid && thread.CloseReasonLocal.String != "")
}

func numbersFromRefs(repo string, refs map[string]struct{}) map[int]struct{} {
	result := make(map[int]struct{})
	prefix := repo + "#"
	for ref := range refs {
		numberText, ok := strings.CutPrefix(ref, prefix)
		if !ok {
			continue
		}
		number, err := strconv.Atoi(numberText)
		if err != nil {
			continue
		}
		result[number] = struct{}{}
	}
	return result
}

func mergeThreads(primary, secondary []Thread) []Thread {
	seen := make(map[string]struct{}, len(primary)+len(secondary))
	merged := make([]Thread, 0, len(primary)+len(secondary))
	for _, thread := range primary {
		key := fmt.Sprintf("%s#%d", thread.Type, thread.Number)
		seen[key] = struct{}{}
		merged = append(merged, thread)
	}
	for _, thread := range secondary {
		key := fmt.Sprintf("%s#%d", thread.Type, thread.Number)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, thread)
	}
	return merged
}

func placeholders(count int) string {
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func PriorityFor(thread Thread, recentWindow time.Duration) int {
	base := 20
	old := 120
	if thread.Type == "pull_request" {
		base = 10
		old = 100
	}
	if !thread.UpdatedAtGH.Valid {
		return old
	}
	updated, err := time.Parse(time.RFC3339, thread.UpdatedAtGH.String)
	if err != nil {
		return old
	}
	if time.Since(updated) <= recentWindow {
		return base
	}
	return old
}

func jobTypeFor(thread Thread) string {
	if thread.Type == "pull_request" {
		return "classify_github_pr"
	}
	return "classify_github_issue"
}

func updatedAt(thread Thread) time.Time {
	value := thread.UpdatedAt
	if thread.UpdatedAtGH.Valid && thread.UpdatedAtGH.String != "" {
		value = thread.UpdatedAtGH.String
	}
	updated, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().UTC()
	}
	return updated.UTC()
}

func sqlNullString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
