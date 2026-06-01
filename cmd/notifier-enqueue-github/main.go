package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/osolmaz/localpager/internal/app"
	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/notifier"
)

func main() {
	flags := enqueueFlags{}

	flag.StringVar(&flags.configPath, "config", "", "JSON config file path")
	app.RegisterSourceCLIFlags(flag.CommandLine, flags.sourceFields(), "source type: prs, issues, or both")
	flag.StringVar(&flags.source, "source", "gitcrawl", "source to enqueue from: gitcrawl or github")
	flag.StringVar(&flags.sourceType, "kind", "both", "deprecated alias for --type")
	flag.IntVar(&flags.limit, "limit", 0, "maximum source items to enqueue")
	flag.BoolVar(&flags.initialHydration, "initial-hydration", false, "record existing items as already handled without classifying them")
	flag.Parse()
	setFlags := app.SeenFlags(flag.CommandLine)

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		log.Fatal(err)
	}
	app.LogConfigWarnings(flags.configPath, cfg)
	flags.applyConfig(cfg, setFlags)

	window := app.ParseDurationFlag("recent-window", flags.recentWindow)
	cutover := app.ParseCutoverFlag(flags.cutoverAt)
	ctx := context.Background()
	pool, err := notifier.NewPool(ctx, flags.dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer app.ClosePool(pool)
	stats, err := app.EnqueueSource(ctx, notifier.NewIngestor(pool), app.SourceOptions{
		Source:           flags.source,
		Repo:             flags.repo,
		Type:             flags.sourceType,
		Limit:            flags.limit,
		InitialHydration: flags.initialHydration,
		ProcessorName:    flags.processorName,
		ProcessorVersion: flags.processorVersion,
		RecentWindow:     window,
		CutoverAt:        cutover,
		GitcrawlDBPath:   flags.gitcrawlDBPath,
		GitHubBaseURL:    flags.githubBaseURL,
		GitHubTokenEnv:   flags.githubTokenEnv,
	})
	if err != nil {
		log.Fatal(err)
	}
	app.Printf(os.Stdout, "%s\n", stats)
}

type enqueueFlags struct {
	configPath       string
	dbPath           string
	gitcrawlDBPath   string
	githubBaseURL    string
	githubTokenEnv   string
	source           string
	repo             string
	sourceType       string
	limit            int
	initialHydration bool
	processorName    string
	processorVersion string
	recentWindow     string
	cutoverAt        string
}

func (flags *enqueueFlags) applyConfig(cfg config.Config, setFlags map[string]bool) {
	app.ApplySourceConfig(flags.sourceFields(), cfg, setFlags, "kind")
	if len(cfg.Watch.Sources) == 1 && !config.FlagSet(setFlags, "source") {
		flags.source = cfg.Watch.Sources[0]
	}
	if cfg.Enqueue.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		flags.limit = cfg.Enqueue.Limit
	}
	if cfg.Enqueue.InitialHydration && !config.FlagSet(setFlags, "initial-hydration") {
		flags.initialHydration = cfg.Enqueue.InitialHydration
	}
}

func (flags *enqueueFlags) sourceFields() app.SourceCLIFields {
	return app.SourceCLIFields{
		DBPath:           &flags.dbPath,
		GitcrawlDBPath:   &flags.gitcrawlDBPath,
		GitHubBaseURL:    &flags.githubBaseURL,
		GitHubTokenEnv:   &flags.githubTokenEnv,
		Repo:             &flags.repo,
		Type:             &flags.sourceType,
		ProcessorName:    &flags.processorName,
		ProcessorVersion: &flags.processorVersion,
		RecentWindow:     &flags.recentWindow,
		CutoverAt:        &flags.cutoverAt,
	}
}
