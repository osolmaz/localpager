package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/osolmaz/localpager/internal/app"
	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/localpager"
)

func main() {
	flags := workerFlags{}

	flag.StringVar(&flags.configPath, "config", "", "JSON config file path")
	flag.StringVar(&flags.dbPath, "db", localpager.DefaultDBPath, "localpager SQLite database path")
	flag.IntVar(&flags.maxConcurrency, "max-concurrency", 2, "maximum concurrent classifier processes")
	flag.StringVar(&flags.leaseTTL, "lease-ttl", "30m", "job lease TTL")
	flag.IntVar(&flags.maxAttempts, "max-attempts", 3, "maximum attempts before marking a job dead")
	flag.IntVar(&flags.limit, "limit", 0, "maximum jobs to process")
	flag.BoolVar(&flags.once, "once", false, "process current work and exit")
	flag.StringVar(&flags.classifierCommand, "classifier-command", localpager.DefaultClassifierCommand, "classifier wrapper command")
	flag.StringVar(&flags.classifierSchema, "classifier-schema", "", "classifier JSON schema path")
	flag.StringVar(&flags.classifierPromptTemplate, "classifier-prompt-template", "", "classifier prompt template path")
	flag.StringVar(&flags.classifierTopicTaxonomy, "classifier-topic-taxonomy", "", "classifier topic taxonomy path")
	flag.StringVar(&flags.githubBaseURL, "github-base-url", "https://api.github.com", "GitHub API base URL for classifier context")
	flag.StringVar(&flags.githubTokenEnv, "github-token-env", "GITHUB_TOKEN", "environment variable containing GitHub token for classifier context")
	flag.StringVar(&flags.model, "model", "", "optional localpager-agent model override")
	flag.StringVar(&flags.discordChannelID, "discord-channel-id", os.Getenv("DISCORD_CHANNEL_ID"), "Discord channel for notifications")
	flag.StringVar(&flags.discordTokenEnv, "discord-token-env", "DISCORD_BOT_TOKEN", "environment variable containing Discord bot token")
	flag.BoolVar(&flags.sendDiscord, "send-discord", false, "send pending Discord notifications")
	flag.BoolVar(&flags.dryRunDiscord, "dry-run-discord", false, "mark Discord sends as dry-run without calling Discord")
	flag.BoolVar(&flags.sendPendingOnly, "send-pending-only", false, "only send pending notifications, do not process jobs")
	flag.StringVar(&flags.pollInterval, "poll-interval", "30s", "idle poll interval for long-running mode")
	flag.Parse()
	setFlags := app.SeenFlags(flag.CommandLine)

	cfg, err := config.Load(flags.configPath)
	if err != nil {
		log.Fatal(err)
	}
	app.LogConfigWarnings(flags.configPath, cfg)
	flags.applyConfig(cfg, setFlags)

	ttl := app.ParseDurationFlag("lease-ttl", flags.leaseTTL)
	pollEvery := app.ParseDurationFlag("poll-interval", flags.pollInterval)
	ctx := context.Background()
	pool, err := localpager.NewPool(ctx, flags.dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer app.ClosePool(pool)

	token := ""
	if flags.sendDiscord && flags.discordChannelID == "" {
		log.Printf("Discord sending enabled but --discord-channel-id/DISCORD_CHANNEL_ID is unset; notifications will remain pending")
	}
	if flags.sendDiscord && !flags.dryRunDiscord {
		token = os.Getenv(flags.discordTokenEnv)
		if token == "" {
			log.Printf("Discord sending enabled but %s is unset; pending notifications will remain pending", flags.discordTokenEnv)
		}
	}
	opts := localpager.WorkerOptions{
		MaxConcurrency:           flags.maxConcurrency,
		LeaseTTL:                 ttl,
		MaxAttempts:              flags.maxAttempts,
		Limit:                    flags.limit,
		Once:                     flags.once,
		ClassifierCommand:        flags.classifierCommand,
		ClassifierSchema:         flags.classifierSchema,
		ClassifierPromptTemplate: flags.classifierPromptTemplate,
		ClassifierTopicTaxonomy:  flags.classifierTopicTaxonomy,
		ClassifierContext:        classifierContextOptions(cfg, flags.githubBaseURL, flags.githubTokenEnv),
		Model:                    flags.model,
		DestinationRef:           flags.discordChannelID,
		DiscordToken:             token,
		SendDiscord:              flags.sendDiscord,
		DryRunDiscord:            flags.dryRunDiscord,
		PollInterval:             pollEvery,
		NotifyTopicsAny:          cfg.Worker.NotifyTopicsAny,
	}
	if flags.sendPendingOnly {
		sent, err := localpager.SendPendingDiscord(ctx, pool, opts)
		if err != nil {
			log.Fatal(err)
		}
		app.Printf(os.Stdout, "sent=%d\n", sent)
		return
	}
	stats, err := localpager.RunWorker(ctx, pool, opts)
	if err != nil {
		log.Fatal(err)
	}
	app.Printf(os.Stdout, "claimed=%d succeeded=%d failed=%d notifications=%d sent=%d\n", stats.Claimed, stats.Succeeded, stats.Failed, stats.Notifications, stats.Sent)
}

type workerFlags struct {
	configPath               string
	dbPath                   string
	maxConcurrency           int
	leaseTTL                 string
	maxAttempts              int
	limit                    int
	once                     bool
	classifierCommand        string
	classifierSchema         string
	classifierPromptTemplate string
	classifierTopicTaxonomy  string
	githubBaseURL            string
	githubTokenEnv           string
	model                    string
	discordChannelID         string
	discordTokenEnv          string
	sendDiscord              bool
	dryRunDiscord            bool
	sendPendingOnly          bool
	pollInterval             string
}

func (flags *workerFlags) applyConfig(cfg config.Config, setFlags map[string]bool) {
	flags.applyCoreConfig(cfg, setFlags)
	flags.applyClassifierConfig(cfg, setFlags)
	flags.applyDiscordConfig(cfg, setFlags)
	flags.applyGitHubConfig(cfg, setFlags)
}

func (flags *workerFlags) applyCoreConfig(cfg config.Config, setFlags map[string]bool) {
	if cfg.DBPath != "" && !config.FlagSet(setFlags, "db") {
		flags.dbPath = cfg.DBPath
	}
	if cfg.Worker.MaxConcurrency != 0 && !config.FlagSet(setFlags, "max-concurrency") {
		flags.maxConcurrency = cfg.Worker.MaxConcurrency
	}
	if cfg.Worker.LeaseTTL != "" && !config.FlagSet(setFlags, "lease-ttl") {
		flags.leaseTTL = cfg.Worker.LeaseTTL
	}
	if cfg.Worker.MaxAttempts != 0 && !config.FlagSet(setFlags, "max-attempts") {
		flags.maxAttempts = cfg.Worker.MaxAttempts
	}
	if cfg.Worker.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		flags.limit = cfg.Worker.Limit
	}
	if cfg.Worker.Once && !config.FlagSet(setFlags, "once") {
		flags.once = cfg.Worker.Once
	}
	if cfg.Worker.PollInterval != "" && !config.FlagSet(setFlags, "poll-interval") {
		flags.pollInterval = cfg.Worker.PollInterval
	}
}

func (flags *workerFlags) applyClassifierConfig(cfg config.Config, setFlags map[string]bool) {
	if cfg.Worker.ClassifierCommand != "" && !config.FlagSet(setFlags, "classifier-command") {
		flags.classifierCommand = cfg.Worker.ClassifierCommand
	}
	if cfg.Classifier.Schema != "" && !config.FlagSet(setFlags, "classifier-schema") {
		flags.classifierSchema = cfg.Classifier.Schema
	}
	if cfg.Classifier.PromptTemplate != "" && !config.FlagSet(setFlags, "classifier-prompt-template") {
		flags.classifierPromptTemplate = cfg.Classifier.PromptTemplate
	}
	if cfg.Classifier.TopicTaxonomy != "" && !config.FlagSet(setFlags, "classifier-topic-taxonomy") {
		flags.classifierTopicTaxonomy = cfg.Classifier.TopicTaxonomy
	}
	if cfg.Worker.Model != "" && !config.FlagSet(setFlags, "model") {
		flags.model = cfg.Worker.Model
	}
}

func (flags *workerFlags) applyDiscordConfig(cfg config.Config, setFlags map[string]bool) {
	if cfg.Worker.DiscordChannelID != "" && !config.FlagSet(setFlags, "discord-channel-id") {
		flags.discordChannelID = cfg.Worker.DiscordChannelID
	}
	if flags.discordChannelID == "" && cfg.Worker.DiscordChannelIDEnv != "" && !config.FlagSet(setFlags, "discord-channel-id") {
		flags.discordChannelID = os.Getenv(cfg.Worker.DiscordChannelIDEnv)
	}
	if cfg.Worker.DiscordTokenEnv != "" && !config.FlagSet(setFlags, "discord-token-env") {
		flags.discordTokenEnv = cfg.Worker.DiscordTokenEnv
	}
	if cfg.Worker.SendDiscord && !config.FlagSet(setFlags, "send-discord") {
		flags.sendDiscord = cfg.Worker.SendDiscord
	}
	if cfg.Worker.DryRunDiscord && !config.FlagSet(setFlags, "dry-run-discord") {
		flags.dryRunDiscord = cfg.Worker.DryRunDiscord
	}
	if cfg.Worker.SendPendingOnly && !config.FlagSet(setFlags, "send-pending-only") {
		flags.sendPendingOnly = cfg.Worker.SendPendingOnly
	}
}

func (flags *workerFlags) applyGitHubConfig(cfg config.Config, setFlags map[string]bool) {
	if cfg.GitHubBaseURL != "" && !config.FlagSet(setFlags, "github-base-url") {
		flags.githubBaseURL = cfg.GitHubBaseURL
	}
	if cfg.GitHubTokenEnv != "" && !config.FlagSet(setFlags, "github-token-env") {
		flags.githubTokenEnv = cfg.GitHubTokenEnv
	}
}

func classifierContextOptions(cfg config.Config, githubBaseURL string, githubTokenEnv string) localpager.ClassifierContextOptions {
	github := cfg.Classifier.Context.GitHub
	opts := localpager.DefaultClassifierContextOptions()
	opts.IncludeBody = boolValue(github.IncludeBody, opts.IncludeBody)
	opts.IncludeLabels = boolValue(github.IncludeLabels, opts.IncludeLabels)
	opts.IncludeComments = boolValue(github.IncludeComments, opts.IncludeComments)
	opts.IncludeChangedFiles = boolValue(github.IncludeChangedFiles, opts.IncludeChangedFiles)
	opts.IncludeDiff = boolValue(github.IncludeDiff, opts.IncludeDiff)
	if github.MaxBodyChars > 0 {
		opts.MaxBodyChars = github.MaxBodyChars
	}
	if github.MaxCommentsChars > 0 {
		opts.MaxCommentsChars = github.MaxCommentsChars
	}
	if github.MaxChangedFilesChars > 0 {
		opts.MaxChangedFilesChars = github.MaxChangedFilesChars
	}
	if github.MaxDiffChars > 0 {
		opts.MaxDiffChars = github.MaxDiffChars
	}
	opts.GitHubBaseURL = githubBaseURL
	if githubTokenEnv == "" {
		githubTokenEnv = "GITHUB_TOKEN"
	}
	opts.GitHubToken = os.Getenv(githubTokenEnv)
	return opts
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
