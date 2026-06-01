package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/config"
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
	var configPath string

	flag.StringVar(&configPath, "config", "", "JSON config file path")
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
	setFlags := seenFlags()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if cfg.DBPath != "" && !config.FlagSet(setFlags, "db") {
		dbPath = cfg.DBPath
	}
	if cfg.GitcrawlDBPath != "" && !config.FlagSet(setFlags, "gitcrawl-db") {
		gitcrawlDBPath = cfg.GitcrawlDBPath
	}
	if len(cfg.Watch.Sources) > 0 && !config.FlagSet(setFlags, "source") {
		sources = append(sources[:0], cfg.Watch.Sources...)
	}
	if cfg.Repo != "" && !config.FlagSet(setFlags, "repo") {
		repo = cfg.Repo
	}
	if cfg.SourceType != "" && !config.FlagSet(setFlags, "type") {
		itemType = cfg.SourceType
	}
	if cfg.Watch.Interval != "" && !config.FlagSet(setFlags, "interval") {
		interval = cfg.Watch.Interval
	}
	if cfg.Watch.Once && !config.FlagSet(setFlags, "once") {
		once = cfg.Watch.Once
	}
	if cfg.Watch.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		limit = cfg.Watch.Limit
	}
	if cfg.ProcessorName != "" && !config.FlagSet(setFlags, "processor-name") {
		processorName = cfg.ProcessorName
	}
	if cfg.ProcessorVersion != "" && !config.FlagSet(setFlags, "processor-version") {
		processorVersion = cfg.ProcessorVersion
	}
	if cfg.RecentWindow != "" && !config.FlagSet(setFlags, "recent-window") {
		recentWindow = cfg.RecentWindow
	}
	if cfg.CutoverAt != "" && !config.FlagSet(setFlags, "cutover-at") {
		cutoverAt = cfg.CutoverAt
	}

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

func seenFlags() map[string]bool {
	flags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		flags[f.Name] = true
	})
	return flags
}
