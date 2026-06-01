package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
	"github.com/osolmaz/localpager/internal/sources/gitcrawl"
)

func main() {
	var dbPath string
	var gitcrawlDBPath string
	var repo string
	var sourceType string
	var limit int
	var initialHydration bool
	var processorName string
	var processorVersion string
	var recentWindow string
	var cutoverAt string

	flag.StringVar(&dbPath, "db", notifier.DefaultDBPath, "notifier SQLite database path")
	flag.StringVar(&gitcrawlDBPath, "gitcrawl-db", gitcrawl.DefaultDBPath, "gitcrawl SQLite database path")
	flag.StringVar(&repo, "repo", notifier.DefaultRepo, "GitHub repo full name")
	flag.StringVar(&sourceType, "type", "both", "source type: prs, issues, or both")
	flag.StringVar(&sourceType, "kind", "both", "deprecated alias for --type")
	flag.IntVar(&limit, "limit", 0, "maximum source items to enqueue")
	flag.BoolVar(&initialHydration, "initial-hydration", false, "record existing items as already handled without classifying them")
	flag.StringVar(&processorName, "processor-name", notifier.DefaultProcessorName, "processor name")
	flag.StringVar(&processorVersion, "processor-version", notifier.DefaultProcessorVer, "processor version")
	flag.StringVar(&recentWindow, "recent-window", "48h", "duration considered recent for priority")
	flag.StringVar(&cutoverAt, "cutover-at", "", "RFC3339 timestamp; items updated before this are recorded as skipped")
	flag.Parse()

	window, err := time.ParseDuration(recentWindow)
	if err != nil {
		log.Fatalf("invalid --recent-window: %v", err)
	}
	var cutover *time.Time
	if cutoverAt != "" {
		parsedCutover, err := time.Parse(time.RFC3339, cutoverAt)
		if err != nil {
			log.Fatalf("invalid --cutover-at: %v", err)
		}
		cutover = &parsedCutover
	}
	ctx := context.Background()
	pool, err := notifier.NewPool(ctx, dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	gitcrawlDB, err := gitcrawl.OpenDB(ctx, gitcrawlDBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer gitcrawlDB.Close()

	stats, err := gitcrawl.Enqueue(ctx, notifier.NewIngestor(pool), gitcrawlDB, gitcrawl.EnqueueOptions{
		Repo:             repo,
		Type:             sourceType,
		Limit:            limit,
		InitialHydration: initialHydration,
		ProcessorName:    processorName,
		ProcessorVersion: processorVersion,
		RecentWindow:     window,
		CutoverAt:        cutover,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "items_seen=%d items_upserted=%d jobs_inserted=%d jobs_skipped=%d jobs_existing=%d\n", stats.ItemsSeen, stats.ItemsUpserted, stats.JobsInserted, stats.JobsSkipped, stats.JobsExisting)
}
