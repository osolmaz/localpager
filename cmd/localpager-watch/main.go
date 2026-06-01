package main

import (
	"context"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/app"
	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/localpager"
	sourcepkg "github.com/osolmaz/localpager/internal/sources"
	"github.com/osolmaz/localpager/internal/timing"
)

func main() {
	flags := watchFlags{}

	flag.StringVar(&flags.configPath, "config", "", "JSON config file path")
	app.RegisterSourceCLIFlags(flag.CommandLine, flags.sourceFields(), "source item type: prs, issues, or both")
	flag.Var(&flags.sources, "source", "source watcher to run; supports gitcrawl and github")
	flag.StringVar(&flags.interval, "interval", "5s", "poll interval")
	flag.BoolVar(&flags.once, "once", false, "poll once and exit")
	flag.IntVar(&flags.limit, "limit", 0, "maximum source items per poll")
	flag.Parse()
	setFlags := app.SeenFlags(flag.CommandLine)

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		log.Fatal(err)
	}
	app.LogConfigWarnings(flags.configPath, cfg)
	flags.applyConfig(cfg, setFlags)
	flags.defaultSources()

	pollEvery := app.ParseDurationFlag("interval", flags.interval)
	window := app.ParseDurationFlag("recent-window", flags.recentWindow)
	cutover := app.ParseCutoverFlag(flags.cutoverAt)

	ctx := context.Background()
	pool, err := localpager.NewPool(ctx, flags.dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer app.ClosePool(pool)
	ingestor := localpager.NewIngestor(pool)

	for {
		total := sourcepkg.EnqueueStats{}
		for _, source := range flags.sources {
			stats, err := app.EnqueueSource(ctx, ingestor, app.SourceOptions{
				Source:           source,
				Repo:             flags.repo,
				Type:             flags.itemType,
				Limit:            flags.limit,
				ProcessorName:    flags.processorName,
				ProcessorVersion: flags.processorVersion,
				RecentWindow:     window,
				CutoverAt:        cutover,
				GitcrawlDBPath:   flags.gitcrawlDBPath,
				GitHubBaseURL:    flags.githubBaseURL,
				GitHubTokenEnv:   flags.githubTokenEnv,
			})
			if err != nil {
				log.Printf("source=%s error=%v", source, err)
				_ = localpager.RecordWatcherError(ctx, pool, source, watcherName(flags.repo), err)
				if flags.once {
					os.Exit(1)
				}
				continue
			}
			_ = localpager.RecordWatcherSuccess(ctx, pool, source, watcherName(flags.repo), time.Now().UTC().Format(time.RFC3339Nano))
			total.Add(stats)
		}
		app.Printf(os.Stdout, "%s\n", total)
		if flags.once {
			return
		}
		if err := timing.SleepContext(ctx, pollEvery); err != nil {
			log.Fatal(err)
		}
	}
}

type watchFlags struct {
	configPath       string
	dbPath           string
	gitcrawlDBPath   string
	githubBaseURL    string
	githubTokenEnv   string
	sources          app.MultiFlag
	repo             string
	itemType         string
	interval         string
	once             bool
	limit            int
	processorName    string
	processorVersion string
	recentWindow     string
	cutoverAt        string
}

func (flags *watchFlags) applyConfig(cfg config.Config, setFlags map[string]bool) {
	flags.applySourceConfig(cfg, setFlags)
	flags.applyWatchConfig(cfg, setFlags)
}

func (flags *watchFlags) applySourceConfig(cfg config.Config, setFlags map[string]bool) {
	app.ApplySourceConfig(flags.sourceFields(), cfg, setFlags)
}

func (flags *watchFlags) applyWatchConfig(cfg config.Config, setFlags map[string]bool) {
	if len(cfg.Watch.Sources) > 0 && !config.FlagSet(setFlags, "source") {
		flags.sources = append(flags.sources[:0], cfg.Watch.Sources...)
	}
	if cfg.Watch.Interval != "" && !config.FlagSet(setFlags, "interval") {
		flags.interval = cfg.Watch.Interval
	}
	if cfg.Watch.Once && !config.FlagSet(setFlags, "once") {
		flags.once = cfg.Watch.Once
	}
	if cfg.Watch.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		flags.limit = cfg.Watch.Limit
	}
	flags.applyProcessingConfig(cfg, setFlags)
}

func (flags *watchFlags) applyProcessingConfig(cfg config.Config, setFlags map[string]bool) {
	if cfg.ProcessorName != "" && !config.FlagSet(setFlags, "processor-name") {
		flags.processorName = cfg.ProcessorName
	}
	if cfg.ProcessorVersion != "" && !config.FlagSet(setFlags, "processor-version") {
		flags.processorVersion = cfg.ProcessorVersion
	}
	if cfg.RecentWindow != "" && !config.FlagSet(setFlags, "recent-window") {
		flags.recentWindow = cfg.RecentWindow
	}
	if cfg.CutoverAt != "" && !config.FlagSet(setFlags, "cutover-at") {
		flags.cutoverAt = cfg.CutoverAt
	}
}

func (flags *watchFlags) defaultSources() {
	if len(flags.sources) == 0 {
		flags.sources = append(flags.sources, "gitcrawl")
	}
}

func watcherName(repo string) string {
	if strings.TrimSpace(repo) == "" {
		return "default"
	}
	return repo
}

func (flags *watchFlags) sourceFields() app.SourceCLIFields {
	fields := app.SourceCLIFields{}
	fields.DBPath = &flags.dbPath
	fields.GitcrawlDBPath = &flags.gitcrawlDBPath
	fields.GitHubBaseURL = &flags.githubBaseURL
	fields.GitHubTokenEnv = &flags.githubTokenEnv
	fields.Repo = &flags.repo
	fields.Type = &flags.itemType
	fields.ProcessorName = &flags.processorName
	fields.ProcessorVersion = &flags.processorVersion
	fields.RecentWindow = &flags.recentWindow
	fields.CutoverAt = &flags.cutoverAt
	return fields
}
