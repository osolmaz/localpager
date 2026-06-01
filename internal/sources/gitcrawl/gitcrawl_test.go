package gitcrawl

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
)

func TestEnqueueMapsGitcrawlRowsThroughGenericIngest(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := notifier.NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	gitcrawlDB := createFixture(t, filepath.Join(dir, "gitcrawl.sqlite"))
	defer gitcrawlDB.Close()

	stats, err := Enqueue(ctx, notifier.NewIngestor(pool), gitcrawlDB, EnqueueOptions{
		Repo:         "example/repo",
		Type:         "prs",
		RecentWindow: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 1 || stats.ItemsUpserted != 1 || stats.JobsInserted != 1 {
		t.Fatalf("stats = %+v, want one item and one job", stats)
	}

	var item notifier.Item
	if err := pool.GORM().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Source == nil || *item.Source != "gitcrawl" {
		t.Fatalf("item source = %v, want gitcrawl", item.Source)
	}
	if item.Type == nil || *item.Type != "github_pr" {
		t.Fatalf("item type = %v, want github_pr", item.Type)
	}
	if item.Ref == nil || *item.Ref != "example/repo#80568" {
		t.Fatalf("item ref = %v, want example/repo#80568", item.Ref)
	}
}

func TestEnqueueSkipsSuppressedGitcrawlRows(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := notifier.NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	gitcrawlDB := createFixture(t, filepath.Join(dir, "gitcrawl.sqlite"))
	defer gitcrawlDB.Close()
	if _, err := gitcrawlDB.Exec("UPDATE threads SET closed_at_local = '2026-05-20T12:00:00Z', close_reason_local = 'read'"); err != nil {
		t.Fatal(err)
	}

	stats, err := Enqueue(ctx, notifier.NewIngestor(pool), gitcrawlDB, EnqueueOptions{
		Repo: "example/repo",
		Type: "prs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 1 || stats.JobsInserted != 0 {
		t.Fatalf("stats = %+v, want seen suppressed item but no jobs", stats)
	}
}

func TestEnqueueCancelsPendingJobWhenGitcrawlRowCloses(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := notifier.NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	gitcrawlDB := createFixture(t, filepath.Join(dir, "gitcrawl.sqlite"))
	defer gitcrawlDB.Close()
	ingestor := notifier.NewIngestor(pool)

	stats, err := Enqueue(ctx, ingestor, gitcrawlDB, EnqueueOptions{
		Repo: "example/repo",
		Type: "prs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 1 || stats.JobsInserted != 1 {
		t.Fatalf("initial stats = %+v, want one pending job", stats)
	}
	if _, err := gitcrawlDB.Exec(`
UPDATE threads
SET state = 'closed',
    content_hash = 'hash-closed',
    updated_at_gh = '2026-05-20T01:00:00Z',
    updated_at = '2026-05-20T01:00:00Z'
WHERE number = 80568`); err != nil {
		t.Fatal(err)
	}

	stats, err = Enqueue(ctx, ingestor, gitcrawlDB, EnqueueOptions{
		Repo: "example/repo",
		Type: "prs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 1 || stats.ItemsUpserted != 1 {
		t.Fatalf("closed stats = %+v, want closed active item re-ingested", stats)
	}

	var item notifier.Item
	if err := pool.GORM().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.State == nil || *item.State != "closed" {
		t.Fatalf("item state = %v, want closed", item.State)
	}
	if item.LatestContentHash == nil || *item.LatestContentHash != "hash-closed" {
		t.Fatalf("latest content hash = %v, want hash-closed", item.LatestContentHash)
	}
	var job notifier.Job
	if err := pool.GORM().Where("content_hash = ?", "hash-1").First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.LastError == nil || *job.LastError != "source_suppressed" {
		t.Fatalf("job last_error = %v, want source_suppressed", job.LastError)
	}
}

func TestEnqueueSuppressesPendingNotificationWhenGitcrawlRowCloses(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := notifier.NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	gitcrawlDB := createFixture(t, filepath.Join(dir, "gitcrawl.sqlite"))
	defer gitcrawlDB.Close()
	ingestor := notifier.NewIngestor(pool)

	if _, err := Enqueue(ctx, ingestor, gitcrawlDB, EnqueueOptions{
		Repo: "example/repo",
		Type: "prs",
	}); err != nil {
		t.Fatal(err)
	}
	var item notifier.Item
	if err := pool.GORM().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	var job notifier.Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := pool.GORM().Model(&notifier.Job{}).Where("id = ?", job.ID).Update("status", "succeeded").Error; err != nil {
		t.Fatal(err)
	}
	result := notifier.Result{
		ItemID:     item.ID,
		JobID:      job.ID,
		JobKind:    job.JobKind,
		OutputJSON: "{}",
	}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	notification := notifier.Notification{
		ItemID:           item.ID,
		ResultID:         result.ID,
		JobID:            job.ID,
		NotificationKind: "github_interest",
		DestinationKind:  "discord_channel",
		DestinationRef:   "channel",
		MessageKey:       "message-key",
		MessageBody:      "message body",
		Status:           "pending",
	}
	if err := pool.GORM().Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := gitcrawlDB.Exec(`
UPDATE threads
SET state = 'closed',
    updated_at = '2026-05-20T01:00:00Z'
WHERE number = 80568`); err != nil {
		t.Fatal(err)
	}

	stats, err := Enqueue(ctx, ingestor, gitcrawlDB, EnqueueOptions{
		Repo: "example/repo",
		Type: "prs",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 1 || stats.ItemsUpserted != 1 {
		t.Fatalf("closed stats = %+v, want closed item re-ingested for pending notification", stats)
	}

	var updated notifier.Notification
	if err := pool.GORM().First(&updated, notification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "sent" {
		t.Fatalf("notification status = %q, want sent", updated.Status)
	}
	if updated.SuppressionReason == nil || *updated.SuppressionReason != "source_suppressed" {
		t.Fatalf("suppression = %v, want source_suppressed", updated.SuppressionReason)
	}
	if updated.ExternalMessageID != nil {
		t.Fatalf("external message id = %v, want nil", updated.ExternalMessageID)
	}
}

func createFixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
CREATE TABLE repositories (
  id integer primary key,
  full_name text not null unique
);
CREATE TABLE threads (
  id integer primary key,
  repo_id integer not null,
  number integer not null,
  kind text not null,
  state text not null,
  title text not null,
  author_login text,
  html_url text not null,
  content_hash text not null,
  updated_at_gh text,
  closed_at_local text,
  close_reason_local text,
  updated_at text not null
);
INSERT INTO repositories (id, full_name) VALUES (1, 'example/repo');
INSERT INTO threads (
  repo_id, number, kind, state, title, author_login, html_url, content_hash, updated_at_gh, updated_at
) VALUES (
  1, 80568, 'pull_request', 'open', 'Example pull request title',
  'example-user', 'https://github.com/example/repo/pull/80568', 'hash-1',
  '2026-05-20T00:00:00Z', '2026-05-20T00:00:00Z'
);
`)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}
