package notifier

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSQLitePoolWaitsForConcurrentWriter(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "notifier.sqlite")
	pool, err := NewPool(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	locker, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		t.Fatal(err)
	}
	defer locker.Close()
	if _, err := locker.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- pool.GORM().Create(&Item{
			Source:            stringPtr("test"),
			Type:              stringPtr("github_pr"),
			Ref:               stringPtr("example/repo#locked"),
			SourceKind:        "github_pr",
			SourceRef:         "example/repo#locked",
			LatestContentHash: stringPtr("hash"),
		}).Error
	}()

	select {
	case err := <-done:
		t.Fatalf("write completed before lock release: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if _, err := locker.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("write did not complete after lock release")
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("write did not wait for lock, elapsed=%s", elapsed)
	}
}

func TestInitialHydrationSkipsClassification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	result, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType:          "classify_github_pr",
		InitialHydration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobInserted {
		t.Fatalf("JobInserted = false, want true")
	}
	if !result.JobSkipped {
		t.Fatalf("JobSkipped = false, want true")
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.NotificationSuppressionReason == nil || *job.NotificationSuppressionReason != "initial_hydration" {
		t.Fatalf("suppression = %v, want initial_hydration", job.NotificationSuppressionReason)
	}

	workerStats, err := RunWorker(ctx, pool, WorkerOptions{
		Once:              true,
		MaxConcurrency:    2,
		ClassifierCommand: filepath.Join(dir, "missing-classifier"),
		SendDiscord:       true,
		DryRunDiscord:     true,
		DiscordToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if workerStats.Claimed != 0 || workerStats.Succeeded != 0 {
		t.Fatalf("worker stats = %+v, want no claimed or succeeded jobs", workerStats)
	}

	var resultCount int64
	if err := pool.GORM().Model(&Result{}).Count(&resultCount).Error; err != nil {
		t.Fatal(err)
	}
	if resultCount != 0 {
		t.Fatalf("result count = %d, want 0", resultCount)
	}
	var notificationCount int64
	if err := pool.GORM().Model(&Notification{}).Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 0 {
		t.Fatalf("notification count = %d, want 0", notificationCount)
	}
}

func TestCutoverSkipsOlderItems(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	cutoverAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	result, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType:   "classify_github_pr",
		CutoverAt: &cutoverAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobInserted {
		t.Fatalf("JobInserted = false, want true")
	}
	if !result.JobSkipped {
		t.Fatalf("JobSkipped = false, want true")
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.NotificationSuppressionReason == nil || *job.NotificationSuppressionReason != "pre_cutover" {
		t.Fatalf("suppression = %v, want pre_cutover", job.NotificationSuppressionReason)
	}
}

func TestCutoverUsesGenericItemUpdatedAt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	cutoverAt := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	item := githubPRItem()
	item.UpdatedAt = time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	result, err := Ingest(ctx, pool, item, IngestOptions{
		JobType:   "classify_github_pr",
		CutoverAt: &cutoverAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobInserted {
		t.Fatalf("JobInserted = false, want true")
	}
	if !result.JobSkipped {
		t.Fatalf("JobSkipped = false, want true")
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.NotificationSuppressionReason == nil || *job.NotificationSuppressionReason != "pre_cutover" {
		t.Fatalf("suppression = %v, want pre_cutover", job.NotificationSuppressionReason)
	}
}

func TestSuppressionDoesNotReskipExistingQueuedJob(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	result, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType:          "classify_github_pr",
		InitialHydration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobExisting || result.JobInserted || result.JobSkipped {
		t.Fatalf("result = %+v, want existing job left queued", result)
	}
	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "pending" {
		t.Fatalf("job status = %q, want pending", job.Status)
	}
	if job.NotificationSuppressionReason != nil {
		t.Fatalf("suppression = %v, want nil", job.NotificationSuppressionReason)
	}
}

func TestSuppressedSourceItemsAreNotEnqueued(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := githubPRItem()
	item.Suppressed = true
	result, err := Ingest(ctx, pool, item, IngestOptions{
		JobType: "classify_github_pr",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Suppressed || result.JobInserted {
		t.Fatalf("result = %+v, want suppressed item with no job", result)
	}
}

func TestSuppressedSourceItemsSkipExistingPendingJobs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType: "classify_github_pr",
	}); err != nil {
		t.Fatal(err)
	}
	item := githubPRItem()
	item.Suppressed = true
	if _, err := Ingest(ctx, pool, item, IngestOptions{
		JobType: "classify_github_pr",
	}); err != nil {
		t.Fatal(err)
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.LastError == nil || *job.LastError != "source_suppressed" {
		t.Fatalf("last error = %v, want source_suppressed", job.LastError)
	}
}

func TestSuppressedSourceItemsSkipExistingRunningJobs(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	result, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}
	leaseUntil := time.Now().UTC().Add(time.Minute)
	if err := pool.GORM().Model(&Job{}).Where("item_id = ?", result.ItemID).Updates(map[string]any{
		"status":       "running",
		"leased_until": &leaseUntil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	item := githubPRItem()
	item.Suppressed = true
	if _, err := Ingest(ctx, pool, item, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", job.Status)
	}
	if job.LeasedUntil != nil {
		t.Fatalf("LeasedUntil = %v, want nil", job.LeasedUntil)
	}
	if job.LastError == nil || *job.LastError != "source_suppressed" {
		t.Fatalf("last error = %v, want source_suppressed", job.LastError)
	}
}

func TestUnsuppressedSourceItemRequeuesSkippedSuppressionJob(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if _, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	suppressed := githubPRItem()
	suppressed.Suppressed = true
	if _, err := Ingest(ctx, pool, suppressed, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	result, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType:  "classify_github_pr",
		Priority: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobInserted || result.JobExisting {
		t.Fatalf("result = %+v, want reactivated pending job", result)
	}

	var job Job
	if err := pool.GORM().First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "pending" {
		t.Fatalf("job status = %q, want pending", job.Status)
	}
	if job.LastError != nil {
		t.Fatalf("last error = %v, want nil", job.LastError)
	}
	if job.Priority != 7 {
		t.Fatalf("priority = %d, want 7", job.Priority)
	}
}

func TestRevertedSupersededContentHashRequeuesSkippedJob(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	first := githubPRItem()
	first.ContentHash = "hash-a"
	if _, err := Ingest(ctx, pool, first, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	second := githubPRItem()
	second.ContentHash = "hash-b"
	if _, err := Ingest(ctx, pool, second, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	if err := skipSupersededPendingJobs(ctx, pool.GORM(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	result, err := Ingest(ctx, pool, first, IngestOptions{
		JobType:  "classify_github_pr",
		Priority: 6,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.JobInserted || result.JobExisting {
		t.Fatalf("result = %+v, want reverted superseded job requeued", result)
	}

	var job Job
	if err := pool.GORM().Where("content_hash = ?", "hash-a").First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "pending" {
		t.Fatalf("job status = %q, want pending", job.Status)
	}
	if job.LastError != nil {
		t.Fatalf("last error = %v, want nil", job.LastError)
	}
	if job.Priority != 6 {
		t.Fatalf("priority = %d, want 6", job.Priority)
	}
}

func TestClaimJobsIgnoresSupersededContentHashWithoutSweep(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	first := githubPRItem()
	first.ContentHash = "hash-a"
	if _, err := Ingest(ctx, pool, first, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	second := githubPRItem()
	second.ContentHash = "hash-b"
	if _, err := Ingest(ctx, pool, second, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	claimed, err := claimJobs(ctx, pool.GORM(), 1, time.Minute, 3, -1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want 1", len(claimed))
	}
	if claimed[0].ContentHash != "hash-b" {
		t.Fatalf("claimed content hash = %q, want hash-b", claimed[0].ContentHash)
	}

	var stale Job
	if err := pool.GORM().Where("content_hash = ?", "hash-a").First(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if stale.Status != "pending" {
		t.Fatalf("stale job status = %q, want pending until sweep", stale.Status)
	}
}

func TestGenericIngestAllowsSameTypeAndRefFromDifferentSources(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	first := githubPRItem()
	first.Source = "gitcrawl"
	second := githubPRItem()
	second.Source = "discord_store"

	if _, err := Ingest(ctx, pool, first, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Ingest(ctx, pool, second, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := pool.GORM().Model(&Item{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("item count = %d, want 2", count)
	}
}

func TestFallbackContentHashIsStableWhenUpdatedAtIsOmitted(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := githubPRItem()
	item.UpdatedAt = time.Time{}
	item.ContentHash = ""
	first, err := Ingest(ctx, pool, item, IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}
	if !first.JobInserted {
		t.Fatalf("first JobInserted = false, want true")
	}
	second, err := Ingest(ctx, pool, item, IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}
	if !second.JobExisting || second.JobInserted {
		t.Fatalf("second result = %+v, want existing unchanged job", second)
	}
	var count int64
	if err := pool.GORM().Model(&Job{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("job count = %d, want 1", count)
	}
}

func TestFallbackContentHashNormalizesEquivalentUpdatedAtOffsets(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	first := githubPRItem()
	first.ContentHash = ""
	first.UpdatedAt = time.Date(2026, 5, 20, 0, 0, 0, 0, time.FixedZone("SGT", 8*60*60))
	second := githubPRItem()
	second.ContentHash = ""
	second.UpdatedAt = time.Date(2026, 5, 19, 16, 0, 0, 0, time.UTC)

	initial, err := Ingest(ctx, pool, first, IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}
	if !initial.JobInserted {
		t.Fatalf("initial JobInserted = false, want true")
	}
	reingest, err := Ingest(ctx, pool, second, IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}
	if !reingest.JobExisting || reingest.JobInserted {
		t.Fatalf("reingest result = %+v, want existing unchanged job", reingest)
	}
	var count int64
	if err := pool.GORM().Model(&Job{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("job count = %d, want 1", count)
	}
}

func TestStaleIngestDoesNotRegressLatestContentPointer(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	newer := githubPRItem()
	newer.Title = "newer title"
	newer.ContentHash = "hash-new"
	newer.UpdatedAt = time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	if _, err := Ingest(ctx, pool, newer, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	stale := githubPRItem()
	stale.Title = "stale title"
	stale.ContentHash = "hash-old"
	stale.UpdatedAt = time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	if _, err := Ingest(ctx, pool, stale, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	var item Item
	if err := pool.GORM().First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.LatestContentHash == nil || *item.LatestContentHash != "hash-new" {
		t.Fatalf("latest content hash = %v, want hash-new", item.LatestContentHash)
	}
	if item.Title == nil || *item.Title != "newer title" {
		t.Fatalf("title = %v, want newer title", item.Title)
	}
	if item.SourceUpdatedAt == nil || !item.SourceUpdatedAt.Equal(newer.UpdatedAt) {
		t.Fatalf("source updated at = %v, want %v", item.SourceUpdatedAt, newer.UpdatedAt)
	}

	claimed, err := claimJobs(ctx, pool.GORM(), 2, time.Minute, 3, -1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want only current job", len(claimed))
	}
	if claimed[0].ContentHash != "hash-new" {
		t.Fatalf("claimed content hash = %q, want hash-new", claimed[0].ContentHash)
	}
}

func TestStaleSuppressedIngestDoesNotCancelCurrentWork(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	newer := githubPRItem()
	newer.Title = "newer active title"
	newer.ContentHash = "hash-new"
	newer.UpdatedAt = time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	ingestResult, err := Ingest(ctx, pool, newer, IngestOptions{JobType: "classify_github_pr"})
	if err != nil {
		t.Fatal(err)
	}

	var job Job
	if err := pool.GORM().Where("item_id = ?", ingestResult.ItemID).First(&job).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	classification := Result{
		ItemID:     ingestResult.ItemID,
		JobID:      job.ID,
		JobKind:    job.JobKind,
		OutputJSON: `{}`,
		CreatedAt:  now,
	}
	if err := pool.GORM().Create(&classification).Error; err != nil {
		t.Fatal(err)
	}
	notification := Notification{
		ItemID:           ingestResult.ItemID,
		ResultID:         classification.ID,
		JobID:            job.ID,
		NotificationKind: "github_interest",
		DestinationKind:  "discord",
		DestinationRef:   "test-channel",
		MessageKey:       "test-message",
		MessageBody:      "current notification",
		Status:           "pending",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := pool.GORM().Create(&notification).Error; err != nil {
		t.Fatal(err)
	}

	stale := githubPRItem()
	stale.Title = "stale closed title"
	stale.State = "closed"
	stale.ContentHash = "hash-old"
	stale.UpdatedAt = time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	stale.Suppressed = true
	if _, err := Ingest(ctx, pool, stale, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	var updatedJob Job
	if err := pool.GORM().First(&updatedJob, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedJob.Status != "pending" {
		t.Fatalf("job status = %q, want pending", updatedJob.Status)
	}
	if updatedJob.LastError != nil {
		t.Fatalf("job last error = %v, want nil", updatedJob.LastError)
	}
	var updatedNotification Notification
	if err := pool.GORM().First(&updatedNotification, notification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedNotification.Status != "pending" {
		t.Fatalf("notification status = %q, want pending", updatedNotification.Status)
	}
	if updatedNotification.SuppressionReason != nil {
		t.Fatalf("notification suppression = %v, want nil", updatedNotification.SuppressionReason)
	}

	var item Item
	if err := pool.GORM().First(&item, ingestResult.ItemID).Error; err != nil {
		t.Fatal(err)
	}
	if item.LatestContentHash == nil || *item.LatestContentHash != "hash-new" {
		t.Fatalf("latest content hash = %v, want hash-new", item.LatestContentHash)
	}
	if item.Title == nil || *item.Title != "newer active title" {
		t.Fatalf("title = %v, want newer active title", item.Title)
	}
}

func TestLiveLocalModelResultCreatesDryRunNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ingestResult, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{
		JobType:  "classify_github_pr",
		Priority: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ingestResult.JobInserted {
		t.Fatalf("JobInserted = false, want true")
	}
	if ingestResult.JobSkipped {
		t.Fatalf("JobSkipped = true, want false")
	}

	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
set -euo pipefail
echo 'prompt: /tmp/test.prompt.txt' >&2
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}

	workerStats, err := RunWorker(ctx, pool, WorkerOptions{
		Once:              true,
		MaxConcurrency:    2,
		ClassifierCommand: classifier,
		SendDiscord:       true,
		DryRunDiscord:     true,
		DiscordToken:      "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if workerStats.Succeeded != 1 {
		t.Fatalf("Succeeded = %d, want 1", workerStats.Succeeded)
	}
	var result Result
	if err := pool.GORM().First(&result).Error; err != nil {
		t.Fatal(err)
	}
	if result.Model == nil || *result.Model != fallbackClassifierModel {
		t.Fatalf("result model = %v, want %s", result.Model, fallbackClassifierModel)
	}

	var notification Notification
	if err := pool.GORM().First(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if notification.Status != "sent" {
		t.Fatalf("notification status = %q, want sent", notification.Status)
	}
	if notification.SuppressionReason != nil {
		t.Fatalf("suppression = %v, want nil", notification.SuppressionReason)
	}
	if notification.ExternalMessageID == nil || *notification.ExternalMessageID != "dry-run" {
		t.Fatalf("ExternalMessageID = %v, want dry-run", notification.ExternalMessageID)
	}
}

func TestSourceSuppressionSuppressesPendingNotification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := Ingest(ctx, pool, githubPRItem(), IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}

	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
set -euo pipefail
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}
	workerStats, err := RunWorker(ctx, pool, WorkerOptions{
		Once:              true,
		ClassifierCommand: classifier,
		SendDiscord:       false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if workerStats.Notifications != 1 || workerStats.Sent != 0 {
		t.Fatalf("worker stats = %+v, want one unsent notification", workerStats)
	}

	suppressed := githubPRItem()
	suppressed.Suppressed = true
	if _, err := Ingest(ctx, pool, suppressed, IngestOptions{JobType: "classify_github_pr"}); err != nil {
		t.Fatal(err)
	}
	var notification Notification
	if err := pool.GORM().First(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if notification.Status != "sent" {
		t.Fatalf("notification status = %q, want sent", notification.Status)
	}
	if notification.SuppressionReason == nil || *notification.SuppressionReason != "source_suppressed" {
		t.Fatalf("suppression = %v, want source_suppressed", notification.SuppressionReason)
	}
	if notification.ExternalMessageID != nil {
		t.Fatalf("external message id = %v, want nil", notification.ExternalMessageID)
	}
}

func TestWorkerKeepsConcurrencySlotsFilled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for i := 1; i <= 3; i++ {
		number := strconv.Itoa(i)
		item := githubPRItem()
		item.Ref = "example/repo#" + number
		item.URL = "https://github.com/example/repo/pull/" + number
		item.ContentHash = "hash-slot-fill-" + number
		item.Metadata = map[string]any{
			"repo":   "example/repo",
			"number": i,
		}
		if _, err := Ingest(ctx, pool, item, IngestOptions{JobType: "classify_github_pr"}); err != nil {
			t.Fatal(err)
		}
	}

	logPath := filepath.Join(dir, "classifier.log")
	classifier := filepath.Join(dir, "classifier.sh")
	script := strings.ReplaceAll(`#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  */pull/1)
    echo one-start >> __LOG__
    sleep 0.8
    echo one-end >> __LOG__
    ;;
  */pull/2)
    echo two >> __LOG__
    ;;
  */pull/3)
    echo three-start >> __LOG__
    sleep 0.1
    echo three-end >> __LOG__
    ;;
esac
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i4","topics_of_interest":[],"description":"not interesting"}
JSON
`, "__LOG__", logPath)
	if err := os.WriteFile(classifier, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	stats, err := RunWorker(ctx, pool, WorkerOptions{
		Once:              true,
		MaxConcurrency:    2,
		ClassifierCommand: classifier,
		SendDiscord:       false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Claimed != 3 || stats.Succeeded != 3 {
		t.Fatalf("stats = %+v, want three claimed and succeeded", stats)
	}
	rawLog, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(rawLog)), "\n")
	oneEnd := indexOfString(lines, "one-end")
	threeStart := indexOfString(lines, "three-start")
	if threeStart == -1 || oneEnd == -1 {
		t.Fatalf("classifier log = %v, want one-end and three-start", lines)
	}
	if threeStart > oneEnd {
		t.Fatalf("classifier log = %v, want third job to start before first slow job ends", lines)
	}
}

func TestSuppressedClaimedNotificationIsNotDelivered(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{SourceKind: "github_pr", SourceRef: "example/repo#3"}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Status:           "succeeded",
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	externalID := "claimed-before-suppression"
	notification := Notification{
		ItemID:            item.ID,
		ResultID:          result.ID,
		JobID:             job.ID,
		NotificationKind:  "github_interest",
		DestinationKind:   "discord_channel",
		DestinationRef:    "channel",
		MessageKey:        "suppressed-claimed",
		MessageBody:       "should not send",
		Status:            "sending",
		ExternalMessageID: &externalID,
	}
	if err := pool.GORM().Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if err := skipPendingJobsForItem(ctx, pool.GORM(), item.ID, "source_suppressed"); err != nil {
		t.Fatal(err)
	}

	sent, err := sendClaimedDiscordNotifications(ctx, pool, WorkerOptions{DryRunDiscord: true}, []Notification{notification})
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("sent = %d, want 0", sent)
	}

	var updated Notification
	if err := pool.GORM().First(&updated, notification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "sent" {
		t.Fatalf("status = %q, want sent", updated.Status)
	}
	if updated.SuppressionReason == nil || *updated.SuppressionReason != "source_suppressed" {
		t.Fatalf("suppression = %v, want source_suppressed", updated.SuppressionReason)
	}
	if updated.ExternalMessageID != nil {
		t.Fatalf("external message id = %v, want nil", updated.ExternalMessageID)
	}
}

func TestPermanentDiscordSendFailureDoesNotStarveLaterNotifications(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	originalSend := sendDiscordMessageFunc
	sendDiscordMessageFunc = func(_ context.Context, _, _, content string) (string, error) {
		if content == "bad" {
			return "", &DiscordSendError{StatusCode: 403, Message: "missing access"}
		}
		return "sent-" + content, nil
	}
	defer func() {
		sendDiscordMessageFunc = originalSend
	}()

	item := Item{SourceKind: "github_pr", SourceRef: "example/repo#4"}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Status:           "succeeded",
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().UTC()
	notifications := []Notification{
		{
			ItemID:           item.ID,
			ResultID:         result.ID,
			JobID:            job.ID,
			NotificationKind: "github_interest",
			DestinationKind:  "discord_channel",
			DestinationRef:   "channel",
			MessageKey:       "bad",
			MessageBody:      "bad",
			Status:           "pending",
			CreatedAt:        createdAt.Add(-time.Minute),
		},
		{
			ItemID:           item.ID,
			ResultID:         result.ID,
			JobID:            job.ID,
			NotificationKind: "github_interest",
			DestinationKind:  "discord_channel",
			DestinationRef:   "channel",
			MessageKey:       "good",
			MessageBody:      "good",
			Status:           "pending",
			CreatedAt:        createdAt,
		},
	}
	if err := pool.GORM().Create(&notifications).Error; err != nil {
		t.Fatal(err)
	}

	sent, err := SendPendingDiscord(ctx, pool, WorkerOptions{
		SendDiscord:    true,
		DiscordToken:   "token",
		DestinationRef: "channel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}

	var updated []Notification
	if err := pool.GORM().Order("message_key ASC").Find(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 {
		t.Fatalf("len(updated) = %d, want 2", len(updated))
	}
	if updated[0].MessageKey != "bad" || updated[0].Status != "failed" {
		t.Fatalf("bad notification = %+v, want failed", updated[0])
	}
	if updated[0].LastError == nil || !strings.Contains(*updated[0].LastError, "missing access") {
		t.Fatalf("bad LastError = %v, want missing access", updated[0].LastError)
	}
	if updated[1].MessageKey != "good" || updated[1].Status != "sent" {
		t.Fatalf("good notification = %+v, want sent", updated[1])
	}
	if updated[1].ExternalMessageID == nil || *updated[1].ExternalMessageID != "sent-good" {
		t.Fatalf("good external id = %v, want sent-good", updated[1].ExternalMessageID)
	}
}

func TestManualNotificationResetDryRunsSend(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{SourceKind: "github_pr", SourceRef: "example/repo#80568"}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:                        item.ID,
		JobKind:                       "classify_github_pr",
		ProcessorName:                 "test",
		ProcessorVersion:              "v1",
		ContentHash:                   "hash",
		Status:                        "succeeded",
		NotificationSuppressionReason: stringPtr("initial_hydration"),
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	notification := Notification{
		ItemID:            item.ID,
		ResultID:          result.ID,
		JobID:             job.ID,
		NotificationKind:  "github_interest",
		DestinationKind:   "discord_channel",
		DestinationRef:    "channel",
		MessageKey:        "manual-test",
		MessageBody:       "manual test",
		Status:            "sent",
		SuppressionReason: stringPtr("initial_hydration"),
		SentAt:            &now,
	}
	if err := pool.GORM().Create(&notification).Error; err != nil {
		t.Fatal(err)
	}
	if err := pool.GORM().Model(&Notification{}).Where("id = ?", notification.ID).Updates(map[string]any{
		"status":             "pending",
		"sent_at":            nil,
		"suppression_reason": "manual_test",
		"attempts":           0,
		"last_error":         nil,
	}).Error; err != nil {
		t.Fatal(err)
	}
	sent, err := SendPendingDiscord(ctx, pool, WorkerOptions{
		SendDiscord:   true,
		DryRunDiscord: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	var updated Notification
	if err := pool.GORM().First(&updated, notification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.ExternalMessageID == nil || *updated.ExternalMessageID != "dry-run" {
		t.Fatalf("ExternalMessageID = %v, want dry-run", updated.ExternalMessageID)
	}
}

func TestSendPendingDiscordDrainsMultipleBatches(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{SourceKind: "github_pr", SourceRef: "example/repo#80568"}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	for index := range pendingDiscordBatchSize + 3 {
		job := Job{
			ItemID:           item.ID,
			JobKind:          "classify_github_pr",
			ProcessorName:    "test",
			ProcessorVersion: "v1",
			ContentHash:      fmt.Sprintf("hash-%d", index),
			Status:           "succeeded",
		}
		if err := pool.GORM().Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
		if err := pool.GORM().Create(&result).Error; err != nil {
			t.Fatal(err)
		}
		notification := Notification{
			ItemID:           item.ID,
			ResultID:         result.ID,
			JobID:            job.ID,
			NotificationKind: "github_interest",
			DestinationKind:  "discord_channel",
			DestinationRef:   "channel",
			MessageKey:       fmt.Sprintf("manual-test-%d", index),
			MessageBody:      fmt.Sprintf("manual test %d", index),
			Status:           "pending",
		}
		if err := pool.GORM().Create(&notification).Error; err != nil {
			t.Fatal(err)
		}
	}

	sent, err := SendPendingDiscord(ctx, pool, WorkerOptions{
		SendDiscord:   true,
		DryRunDiscord: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent != pendingDiscordBatchSize+3 {
		t.Fatalf("sent = %d, want %d", sent, pendingDiscordBatchSize+3)
	}
	var pending int64
	if err := pool.GORM().Model(&Notification{}).Where("status = ?", "pending").Count(&pending).Error; err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("pending notifications = %d, want 0", pending)
	}
}

func TestShouldNotifyDefaultsToInterestGate(t *testing.T) {
	cases := []struct {
		name string
		out  ClassifierOutput
		want bool
	}{
		{
			name: "high interest notifies",
			out:  ClassifierOutput{Interest: "high", TopicsOfInterest: []string{"bug"}},
			want: true,
		},
		{
			name: "low interest does not notify",
			out:  ClassifierOutput{Interest: "low", TopicsOfInterest: []string{"bug"}},
			want: false,
		},
		{
			name: "empty interest does not notify",
			out:  ClassifierOutput{Interest: "", TopicsOfInterest: []string{"bug"}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldNotify(tc.out, WorkerOptions{}); got != tc.want {
				t.Fatalf("shouldNotify() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldNotifyCanRequireConfiguredTopics(t *testing.T) {
	opts := WorkerOptions{NotifyTopicsAny: []string{
		"local_models",
		"self_hosted_inference",
		"local_model_providers",
		"open_weight_models",
		"acpx",
	}}
	cases := []struct {
		name string
		out  ClassifierOutput
		want bool
	}{
		{
			name: "local model topic",
			out:  ClassifierOutput{Interest: "i4", TopicsOfInterest: []string{"local_models"}},
			want: true,
		},
		{
			name: "acpx topic",
			out:  ClassifierOutput{Interest: "i4", TopicsOfInterest: []string{"acpx"}},
			want: true,
		},
		{
			name: "model serving alone is not enough",
			out:  ClassifierOutput{Interest: "i0", TopicsOfInterest: []string{"model_serving", "codex", "auth_identity"}},
			want: false,
		},
		{
			name: "model serving with local model topic notifies",
			out:  ClassifierOutput{Interest: "i1", TopicsOfInterest: []string{"model_serving", "local_models"}},
			want: true,
		},
		{
			name: "i0 coding agent topic is not enough",
			out:  ClassifierOutput{Interest: "i0", TopicsOfInterest: []string{"coding_agents", "codex"}},
			want: false,
		},
		{
			name: "i0 without matching topics is not enough",
			out:  ClassifierOutput{Interest: "i0", TopicsOfInterest: []string{}},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldNotify(tc.out, opts); got != tc.want {
				t.Fatalf("shouldNotify() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExpiredFinalAttemptJobIsMarkedDead(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind:        "github_pr",
		SourceRef:         "example/repo#1",
		LatestContentHash: stringPtr("hash"),
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Hour)
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Priority:         1,
		Status:           "running",
		Attempts:         3,
		LeasedUntil:      &expired,
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	claimed, err := claimJobs(ctx, pool.GORM(), 2, time.Minute, 3, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed %d jobs, want 0", len(claimed))
	}
	var updated Job
	if err := pool.GORM().First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "dead" {
		t.Fatalf("status = %q, want dead", updated.Status)
	}
}

func TestClassifierTimeoutUsesLeaseTTL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
sleep 2
`), 0o755); err != nil {
		t.Fatal(err)
	}
	leaseUntil := time.Now().UTC().Add(50 * time.Millisecond)
	_, _, _, _, err := runClassifier(ctx, ClaimedJob{
		Job: Job{LeasedUntil: &leaseUntil},
		Item: Item{
			SourceRef: "example/repo#1",
			SourceURL: stringPtr("https://github.com/example/repo/pull/1"),
		},
	}, WorkerOptions{ClassifierCommand: classifier})
	if err == nil {
		t.Fatal("runClassifier returned nil error, want timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestClassifierUsesGenericRefWhenURLIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
set -euo pipefail
if [[ "$1" != "example/repo#80568" ]]; then
  echo "unexpected target: $1" >&2
  exit 7
fi
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := runClassifier(ctx, ClaimedJob{
		Item: Item{
			Ref:       stringPtr("example/repo#80568"),
			SourceRef: "gitcrawl:example/repo#80568",
		},
	}, WorkerOptions{ClassifierCommand: classifier})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClassifierUsesEmittedSessionPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	classifier := filepath.Join(dir, "classifier.sh")
	sessionPath := filepath.Join(dir, "session.jsonl")
	if err := os.WriteFile(classifier, []byte(fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
echo "prompt: %s" >&2
echo "session: %s" >&2
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`, filepath.Join(dir, "prompt.txt"), sessionPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, promptPath, gotSessionPath, err := runClassifier(ctx, ClaimedJob{
		Item: Item{
			SourceURL: stringPtr("https://github.com/example/repo/pull/1"),
			SourceRef: "example/repo#1",
		},
	}, WorkerOptions{ClassifierCommand: classifier})
	if err != nil {
		t.Fatal(err)
	}
	if promptPath == "" {
		t.Fatal("prompt path is empty")
	}
	if gotSessionPath != sessionPath {
		t.Fatalf("session path = %q, want %q", gotSessionPath, sessionPath)
	}
}

func TestStaleLeaseDoesNotFinalizeResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind: "github_pr",
		SourceRef:  "example/repo#4",
		SourceURL:  stringPtr("https://github.com/example/repo/pull/4"),
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	claimedLease := time.Now().UTC().Add(time.Minute)
	currentLease := time.Now().UTC().Add(2 * time.Minute)
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Status:           "running",
		Attempts:         1,
		LeasedUntil:      &currentLease,
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}

	stats := processJob(ctx, pool, ClaimedJob{
		Job: Job{
			ID:               job.ID,
			ItemID:           item.ID,
			JobKind:          job.JobKind,
			ProcessorName:    job.ProcessorName,
			ProcessorVersion: job.ProcessorVersion,
			ContentHash:      job.ContentHash,
			Status:           "running",
			Attempts:         1,
			LeasedUntil:      &claimedLease,
		},
		Item: item,
	}, WorkerOptions{
		MaxAttempts:       3,
		ClassifierCommand: classifier,
		SendDiscord:       true,
		DryRunDiscord:     true,
	})
	if stats.Succeeded != 0 || stats.Failed != 0 || stats.Notifications != 0 {
		t.Fatalf("stats = %+v, want stale lease to produce no result", stats)
	}
	var resultCount int64
	if err := pool.GORM().Model(&Result{}).Count(&resultCount).Error; err != nil {
		t.Fatal(err)
	}
	if resultCount != 0 {
		t.Fatalf("result count = %d, want 0", resultCount)
	}
	var notificationCount int64
	if err := pool.GORM().Model(&Notification{}).Count(&notificationCount).Error; err != nil {
		t.Fatal(err)
	}
	if notificationCount != 0 {
		t.Fatalf("notification count = %d, want 0", notificationCount)
	}
}

func TestSupersededRunningJobDoesNotFinalizeResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is POSIX-only")
	}
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind:        "github_pr",
		SourceRef:         "example/repo#6",
		SourceURL:         stringPtr("https://github.com/example/repo/pull/6"),
		LatestContentHash: stringPtr("new-hash"),
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	leaseUntil := time.Now().UTC().Add(time.Minute)
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "old-hash",
		Status:           "running",
		Attempts:         1,
		LeasedUntil:      &leaseUntil,
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	classifier := filepath.Join(dir, "classifier.sh")
	if err := os.WriteFile(classifier, []byte(`#!/usr/bin/env bash
cat <<'JSON'
{"caveats":[],"confidence":0.99,"interest":"i1","topics_of_interest":["local_models"],"description":"Local model related change."}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}

	stats := processJob(ctx, pool, ClaimedJob{Job: job, Item: item}, WorkerOptions{
		MaxAttempts:       3,
		ClassifierCommand: classifier,
		SendDiscord:       true,
		DryRunDiscord:     true,
	})
	if stats.Succeeded != 0 || stats.Failed != 0 || stats.Notifications != 0 {
		t.Fatalf("stats = %+v, want superseded job to produce no result", stats)
	}
	var resultCount int64
	if err := pool.GORM().Model(&Result{}).Count(&resultCount).Error; err != nil {
		t.Fatal(err)
	}
	if resultCount != 0 {
		t.Fatalf("result count = %d, want 0", resultCount)
	}
	var updated Job
	if err := pool.GORM().First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "skipped" {
		t.Fatalf("job status = %q, want skipped", updated.Status)
	}
	if updated.LastError == nil || *updated.LastError != "superseded_content_hash" {
		t.Fatalf("last error = %v, want superseded_content_hash", updated.LastError)
	}
}

func TestSupersededPendingNotificationIsSuppressed(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind:        "github_pr",
		SourceRef:         "example/repo#7",
		LatestContentHash: stringPtr("new-hash"),
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "old-hash",
		Status:           "succeeded",
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	notification := Notification{
		ItemID:           item.ID,
		ResultID:         result.ID,
		JobID:            job.ID,
		NotificationKind: "github_interest",
		DestinationKind:  "discord_channel",
		DestinationRef:   "channel",
		MessageKey:       "stale-message",
		MessageBody:      "stale message",
		Status:           "pending",
	}
	if err := pool.GORM().Create(&notification).Error; err != nil {
		t.Fatal(err)
	}

	sent, err := SendPendingDiscord(ctx, pool, WorkerOptions{
		SendDiscord:   true,
		DryRunDiscord: true,
		DiscordToken:  "test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent != 0 {
		t.Fatalf("sent = %d, want 0", sent)
	}
	var updated Notification
	if err := pool.GORM().First(&updated, notification.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != "sent" {
		t.Fatalf("notification status = %q, want sent", updated.Status)
	}
	if updated.SuppressionReason == nil || *updated.SuppressionReason != "superseded_content_hash" {
		t.Fatalf("suppression reason = %v, want superseded_content_hash", updated.SuppressionReason)
	}
	if updated.ExternalMessageID != nil {
		t.Fatalf("external message id = %v, want nil", updated.ExternalMessageID)
	}
}

func TestClaimJobsSkipsSupersededContentHash(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind:        "github_pr",
		SourceRef:         "example/repo#5",
		LatestContentHash: stringPtr("new-hash"),
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	jobs := []Job{
		{
			ItemID:           item.ID,
			JobKind:          "classify_github_pr",
			ProcessorName:    "test",
			ProcessorVersion: "v1",
			ContentHash:      "old-hash",
			Status:           "pending",
			Priority:         1,
		},
		{
			ItemID:           item.ID,
			JobKind:          "classify_github_pr",
			ProcessorName:    "test",
			ProcessorVersion: "v1",
			ContentHash:      "new-hash",
			Status:           "pending",
			Priority:         2,
		},
	}
	if err := pool.GORM().Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := claimJobs(ctx, pool.GORM(), 2, time.Minute, 3, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want 1", len(claimed))
	}
	if claimed[0].ContentHash != "new-hash" {
		t.Fatalf("claimed content hash = %q, want new-hash", claimed[0].ContentHash)
	}
	var old Job
	if err := pool.GORM().First(&old, jobs[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if old.Status != "skipped" {
		t.Fatalf("old job status = %q, want skipped", old.Status)
	}
}

func TestClaimJobsUsesFIFOWithinPriority(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind: "github_pr",
		SourceRef:  "example/repo#fifo",
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	oldCreated := time.Now().UTC().Add(-time.Hour)
	newCreated := time.Now().UTC()
	jobs := []Job{
		{
			ItemID:           item.ID,
			JobKind:          "classify_github_pr",
			ProcessorName:    "test",
			ProcessorVersion: "v1",
			ContentHash:      "old-hash",
			Status:           "pending",
			Priority:         100,
			CreatedAt:        oldCreated,
			UpdatedAt:        oldCreated,
		},
		{
			ItemID:           item.ID,
			JobKind:          "classify_github_pr",
			ProcessorName:    "test",
			ProcessorVersion: "v1",
			ContentHash:      "new-hash",
			Status:           "pending",
			Priority:         100,
			CreatedAt:        newCreated,
			UpdatedAt:        newCreated,
		},
	}
	if err := pool.GORM().Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := claimJobs(ctx, pool.GORM(), 1, time.Minute, 3, -1, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs, want 1", len(claimed))
	}
	if claimed[0].ContentHash != "old-hash" {
		t.Fatalf("claimed content hash = %q, want old-hash", claimed[0].ContentHash)
	}
}

func TestFailedJobUsesRunAfterBackoff(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{
		SourceKind: "github_pr",
		SourceRef:  "example/repo#3",
	}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Priority:         1,
		Status:           "running",
		Attempts:         1,
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := markJobFailed(ctx, pool.GORM(), job.ID, "temporary failure", 3); err != nil {
		t.Fatal(err)
	}
	var failed Job
	if err := pool.GORM().First(&failed, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Status != "pending" {
		t.Fatalf("status = %q, want pending", failed.Status)
	}
	if failed.RunAfter == nil || !failed.RunAfter.After(time.Now().UTC()) {
		t.Fatalf("RunAfter = %v, want future retry time", failed.RunAfter)
	}
	claimed, err := claimJobs(ctx, pool.GORM(), 2, time.Minute, 3, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claimed %d jobs before run_after, want 0", len(claimed))
	}
	past := time.Now().UTC().Add(-time.Minute)
	if err := pool.GORM().Model(&Job{}).Where("id = ?", job.ID).Update("run_after", &past).Error; err != nil {
		t.Fatal(err)
	}
	claimed, err = claimJobs(ctx, pool.GORM(), 2, time.Minute, 3, -1, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimed %d jobs after run_after, want 1", len(claimed))
	}
}

func TestResetUnsentNotificationsReturnsClaimedRowsToPending(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	pool, err := NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	item := Item{SourceKind: "github_pr", SourceRef: "example/repo#2"}
	if err := pool.GORM().Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	job := Job{
		ItemID:           item.ID,
		JobKind:          "classify_github_pr",
		ProcessorName:    "test",
		ProcessorVersion: "v1",
		ContentHash:      "hash",
		Status:           "succeeded",
	}
	if err := pool.GORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	result := Result{ItemID: item.ID, JobID: job.ID, JobKind: job.JobKind, OutputJSON: "{}"}
	if err := pool.GORM().Create(&result).Error; err != nil {
		t.Fatal(err)
	}
	notifications := []Notification{
		{
			ItemID:           item.ID,
			ResultID:         result.ID,
			JobID:            job.ID,
			NotificationKind: "github_interest",
			DestinationKind:  "discord_channel",
			DestinationRef:   "channel",
			MessageKey:       "one",
			MessageBody:      "one",
			Status:           "sending",
		},
		{
			ItemID:           item.ID,
			ResultID:         result.ID,
			JobID:            job.ID,
			NotificationKind: "github_interest",
			DestinationKind:  "discord_channel",
			DestinationRef:   "channel",
			MessageKey:       "two",
			MessageBody:      "two",
			Status:           "sending",
		},
	}
	if err := pool.GORM().Create(&notifications).Error; err != nil {
		t.Fatal(err)
	}

	if err := resetUnsentNotifications(ctx, pool.GORM(), notifications, errors.New("discord unavailable")); err != nil {
		t.Fatal(err)
	}
	var updated []Notification
	if err := pool.GORM().Order("message_key ASC").Find(&updated).Error; err != nil {
		t.Fatal(err)
	}
	if len(updated) != 2 {
		t.Fatalf("len(updated) = %d, want 2", len(updated))
	}
	for _, notification := range updated {
		if notification.Status != "pending" {
			t.Fatalf("notification %s status = %q, want pending", notification.MessageKey, notification.Status)
		}
		if notification.LastError == nil || *notification.LastError != "discord unavailable" {
			t.Fatalf("notification %s LastError = %v, want discord unavailable", notification.MessageKey, notification.LastError)
		}
	}
}

func TestDiscordPayloadSuppressesMentions(t *testing.T) {
	payload, err := json.Marshal(discordMessagePayload{
		Content: "@everyone <@123> local model PR",
		AllowedMentions: discordAllowedMentions{
			Parse: []string{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Content         string `json:"content"`
		AllowedMentions struct {
			Parse []string `json:"parse"`
		} `json:"allowed_mentions"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Content != "@everyone <@123> local model PR" {
		t.Fatalf("Content = %q", decoded.Content)
	}
	if decoded.AllowedMentions.Parse == nil {
		t.Fatal("AllowedMentions.Parse = nil, want empty array")
	}
	if len(decoded.AllowedMentions.Parse) != 0 {
		t.Fatalf("AllowedMentions.Parse = %v, want empty", decoded.AllowedMentions.Parse)
	}
}

func githubPRItem() IngestItem {
	return IngestItem{
		Source:      "test_source",
		Type:        "github_pr",
		Ref:         "example/repo#80568",
		URL:         "https://github.com/example/repo/pull/80568",
		Title:       "Example pull request title",
		State:       "open",
		Author:      "example-user",
		UpdatedAt:   time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
		ContentHash: "hash-1",
		Metadata: map[string]any{
			"repo":   "example/repo",
			"number": 80568,
		},
	}
}

func indexOfString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
