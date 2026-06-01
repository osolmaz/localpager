package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/notifier"
	"github.com/osolmaz/localpager/internal/sources/gitcrawl"
	githubsource "github.com/osolmaz/localpager/internal/sources/github"
)

func main() {
	var dbPath string
	var gitcrawlDBPath string
	var githubBaseURL string
	var githubTokenEnv string
	var source string
	var repo string
	var sourceType string
	var limit int
	var initialHydration bool
	var processorName string
	var processorVersion string
	var recentWindow string
	var cutoverAt string
	var configPath string

	flag.StringVar(&configPath, "config", "", "JSON config file path")
	flag.StringVar(&dbPath, "db", notifier.DefaultDBPath, "notifier SQLite database path")
	flag.StringVar(&gitcrawlDBPath, "gitcrawl-db", gitcrawl.DefaultDBPath, "gitcrawl SQLite database path")
	flag.StringVar(&githubBaseURL, "github-base-url", githubsource.DefaultBaseURL, "GitHub API base URL")
	flag.StringVar(&githubTokenEnv, "github-token-env", "GITHUB_TOKEN", "environment variable containing GitHub token")
	flag.StringVar(&source, "source", "gitcrawl", "source to enqueue from: gitcrawl or github")
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
	setFlags := seenFlags()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatal(err)
	}
	if configPath != "" {
		for _, warning := range cfg.Validate() {
			log.Printf("config warning: %s", warning)
		}
	}
	if cfg.DBPath != "" && !config.FlagSet(setFlags, "db") {
		dbPath = cfg.DBPath
	}
	if cfg.GitcrawlDBPath != "" && !config.FlagSet(setFlags, "gitcrawl-db") {
		gitcrawlDBPath = cfg.GitcrawlDBPath
	}
	if cfg.GitHubBaseURL != "" && !config.FlagSet(setFlags, "github-base-url") {
		githubBaseURL = cfg.GitHubBaseURL
	}
	if cfg.GitHubTokenEnv != "" && !config.FlagSet(setFlags, "github-token-env") {
		githubTokenEnv = cfg.GitHubTokenEnv
	}
	if len(cfg.Watch.Sources) == 1 && !config.FlagSet(setFlags, "source") {
		source = cfg.Watch.Sources[0]
	}
	if cfg.Repo != "" && !config.FlagSet(setFlags, "repo") {
		repo = cfg.Repo
	}
	if cfg.SourceType != "" && !config.FlagSet(setFlags, "type") && !config.FlagSet(setFlags, "kind") {
		sourceType = cfg.SourceType
	}
	if cfg.Enqueue.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		limit = cfg.Enqueue.Limit
	}
	if cfg.Enqueue.InitialHydration && !config.FlagSet(setFlags, "initial-hydration") {
		initialHydration = cfg.Enqueue.InitialHydration
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
	var stats struct {
		ItemsSeen     int
		ItemsUpserted int
		JobsInserted  int
		JobsSkipped   int
		JobsExisting  int
	}
	switch source {
	case "gitcrawl":
		gitcrawlDB, err := gitcrawl.OpenDB(ctx, gitcrawlDBPath)
		if err != nil {
			log.Fatal(err)
		}
		defer gitcrawlDB.Close()
		sourceStats, err := gitcrawl.Enqueue(ctx, notifier.NewIngestor(pool), gitcrawlDB, gitcrawl.EnqueueOptions{
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
		stats.ItemsSeen = sourceStats.ItemsSeen
		stats.ItemsUpserted = sourceStats.ItemsUpserted
		stats.JobsInserted = sourceStats.JobsInserted
		stats.JobsSkipped = sourceStats.JobsSkipped
		stats.JobsExisting = sourceStats.JobsExisting
	case "github":
		sourceStats, err := githubsource.Enqueue(ctx, notifier.NewIngestor(pool), githubsource.EnqueueOptions{
			Repo:             repo,
			Type:             sourceType,
			Limit:            limit,
			InitialHydration: initialHydration,
			ProcessorName:    processorName,
			ProcessorVersion: processorVersion,
			RecentWindow:     window,
			CutoverAt:        cutover,
			BaseURL:          githubBaseURL,
			Token:            os.Getenv(githubTokenEnv),
		})
		if err != nil {
			log.Fatal(err)
		}
		stats.ItemsSeen = sourceStats.ItemsSeen
		stats.ItemsUpserted = sourceStats.ItemsUpserted
		stats.JobsInserted = sourceStats.JobsInserted
		stats.JobsSkipped = sourceStats.JobsSkipped
		stats.JobsExisting = sourceStats.JobsExisting
	default:
		log.Fatalf("unsupported source %q", source)
	}
	fmt.Fprintf(os.Stdout, "items_seen=%d items_upserted=%d jobs_inserted=%d jobs_skipped=%d jobs_existing=%d\n", stats.ItemsSeen, stats.ItemsUpserted, stats.JobsInserted, stats.JobsSkipped, stats.JobsExisting)
}

func seenFlags() map[string]bool {
	flags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		flags[f.Name] = true
	})
	return flags
}
