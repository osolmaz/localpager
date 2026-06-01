package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
	"github.com/osolmaz/localpager/internal/sources/gitcrawl"
)

func main() {
	var dbPath string
	var gitcrawlDBPath string
	var sources multiFlag
	var repo string
	var itemType string
	var interval string
	var once bool
	var limit int
	var processorName string
	var processorVersion string
	var recentWindow string
	var cutoverAt string

	flag.StringVar(&dbPath, "db", notifier.DefaultDBPath, "notifier SQLite database path")
	flag.StringVar(&gitcrawlDBPath, "gitcrawl-db", gitcrawl.DefaultDBPath, "gitcrawl SQLite database path")
	flag.Var(&sources, "source", "source watcher to run; currently supports gitcrawl")
	flag.StringVar(&repo, "repo", notifier.DefaultRepo, "GitHub repo full name for gitcrawl watcher")
	flag.StringVar(&itemType, "type", "both", "gitcrawl item type: prs, issues, or both")
	flag.StringVar(&interval, "interval", "5s", "poll interval")
	flag.BoolVar(&once, "once", false, "poll once and exit")
	flag.IntVar(&limit, "limit", 0, "maximum source items per poll")
	flag.StringVar(&processorName, "processor-name", notifier.DefaultProcessorName, "processor name")
	flag.StringVar(&processorVersion, "processor-version", notifier.DefaultProcessorVer, "processor version")
	flag.StringVar(&recentWindow, "recent-window", "48h", "duration considered recent for priority")
	flag.StringVar(&cutoverAt, "cutover-at", "", "RFC3339 timestamp; items updated before this are recorded as skipped")
	flag.Parse()

	if len(sources) == 0 {
		sources = append(sources, "gitcrawl")
	}
	pollEvery, err := time.ParseDuration(interval)
	if err != nil {
		log.Fatalf("invalid --interval: %v", err)
	}
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
	ingestor := notifier.NewIngestor(pool)

	for {
		total := gitcrawl.EnqueueStats{}
		for _, source := range sources {
			switch source {
			case "gitcrawl":
				stats, err := pollGitcrawl(ctx, ingestor, gitcrawlDBPath, repo, itemType, limit, processorName, processorVersion, window, cutover)
				if err != nil {
					log.Printf("source=%s error=%v", source, err)
					_ = notifier.RecordWatcherError(ctx, pool, source, watcherName(repo), err)
					if once {
						os.Exit(1)
					}
					continue
				}
				_ = notifier.RecordWatcherSuccess(ctx, pool, source, watcherName(repo), time.Now().UTC().Format(time.RFC3339Nano))
				total.ItemsSeen += stats.ItemsSeen
				total.ItemsUpserted += stats.ItemsUpserted
				total.JobsInserted += stats.JobsInserted
				total.JobsSkipped += stats.JobsSkipped
				total.JobsExisting += stats.JobsExisting
			default:
				log.Printf("unsupported source %q", source)
				if once {
					os.Exit(1)
				}
			}
		}
		fmt.Fprintf(os.Stdout, "items_seen=%d items_upserted=%d jobs_inserted=%d jobs_skipped=%d jobs_existing=%d\n", total.ItemsSeen, total.ItemsUpserted, total.JobsInserted, total.JobsSkipped, total.JobsExisting)
		if once {
			return
		}
		if err := sleepContext(ctx, pollEvery); err != nil {
			log.Fatal(err)
		}
	}
}

func watcherName(repo string) string {
	if strings.TrimSpace(repo) == "" {
		return "default"
	}
	return repo
}

func pollGitcrawl(ctx context.Context, ingestor notifier.Ingestor, dbPath, repo, itemType string, limit int, processorName, processorVersion string, recentWindow time.Duration, cutoverAt *time.Time) (gitcrawl.EnqueueStats, error) {
	db, err := gitcrawl.OpenDB(ctx, dbPath)
	if err != nil {
		return gitcrawl.EnqueueStats{}, err
	}
	defer db.Close()
	return gitcrawl.Enqueue(ctx, ingestor, db, gitcrawl.EnqueueOptions{
		Repo:             repo,
		Type:             itemType,
		Limit:            limit,
		ProcessorName:    processorName,
		ProcessorVersion: processorVersion,
		RecentWindow:     recentWindow,
		CutoverAt:        cutoverAt,
	})
}

func sleepContext(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type multiFlag []string

func (values *multiFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *multiFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}
