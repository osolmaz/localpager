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
)

func main() {
	var dbPath string
	var maxConcurrency int
	var leaseTTL string
	var maxAttempts int
	var limit int
	var once bool
	var classifierCommand string
	var model string
	var discordChannelID string
	var discordTokenEnv string
	var sendDiscord bool
	var dryRunDiscord bool
	var sendPendingOnly bool
	var pollInterval string
	var configPath string

	flag.StringVar(&configPath, "config", "", "JSON config file path")
	flag.StringVar(&dbPath, "db", notifier.DefaultDBPath, "notifier SQLite database path")
	flag.IntVar(&maxConcurrency, "max-concurrency", 2, "maximum concurrent classifier processes")
	flag.StringVar(&leaseTTL, "lease-ttl", "30m", "job lease TTL")
	flag.IntVar(&maxAttempts, "max-attempts", 3, "maximum attempts before marking a job dead")
	flag.IntVar(&limit, "limit", 0, "maximum jobs to process")
	flag.BoolVar(&once, "once", false, "process current work and exit")
	flag.StringVar(&classifierCommand, "classifier-command", notifier.DefaultClassifierCommand, "classifier wrapper command")
	flag.StringVar(&model, "model", "", "optional localpager-agent model override")
	flag.StringVar(&discordChannelID, "discord-channel-id", os.Getenv("DISCORD_CHANNEL_ID"), "Discord channel for notifications")
	flag.StringVar(&discordTokenEnv, "discord-token-env", "DISCORD_BOT_TOKEN", "environment variable containing Discord bot token")
	flag.BoolVar(&sendDiscord, "send-discord", false, "send pending Discord notifications")
	flag.BoolVar(&dryRunDiscord, "dry-run-discord", false, "mark Discord sends as dry-run without calling Discord")
	flag.BoolVar(&sendPendingOnly, "send-pending-only", false, "only send pending notifications, do not process jobs")
	flag.StringVar(&pollInterval, "poll-interval", "30s", "idle poll interval for long-running mode")
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
	if cfg.Worker.MaxConcurrency != 0 && !config.FlagSet(setFlags, "max-concurrency") {
		maxConcurrency = cfg.Worker.MaxConcurrency
	}
	if cfg.Worker.LeaseTTL != "" && !config.FlagSet(setFlags, "lease-ttl") {
		leaseTTL = cfg.Worker.LeaseTTL
	}
	if cfg.Worker.MaxAttempts != 0 && !config.FlagSet(setFlags, "max-attempts") {
		maxAttempts = cfg.Worker.MaxAttempts
	}
	if cfg.Worker.Limit != 0 && !config.FlagSet(setFlags, "limit") {
		limit = cfg.Worker.Limit
	}
	if cfg.Worker.Once && !config.FlagSet(setFlags, "once") {
		once = cfg.Worker.Once
	}
	if cfg.Worker.ClassifierCommand != "" && !config.FlagSet(setFlags, "classifier-command") {
		classifierCommand = cfg.Worker.ClassifierCommand
	}
	if cfg.Worker.Model != "" && !config.FlagSet(setFlags, "model") {
		model = cfg.Worker.Model
	}
	if cfg.Worker.DiscordChannelID != "" && !config.FlagSet(setFlags, "discord-channel-id") {
		discordChannelID = cfg.Worker.DiscordChannelID
	}
	if discordChannelID == "" && cfg.Worker.DiscordChannelIDEnv != "" && !config.FlagSet(setFlags, "discord-channel-id") {
		discordChannelID = os.Getenv(cfg.Worker.DiscordChannelIDEnv)
	}
	if cfg.Worker.DiscordTokenEnv != "" && !config.FlagSet(setFlags, "discord-token-env") {
		discordTokenEnv = cfg.Worker.DiscordTokenEnv
	}
	if cfg.Worker.SendDiscord && !config.FlagSet(setFlags, "send-discord") {
		sendDiscord = cfg.Worker.SendDiscord
	}
	if cfg.Worker.DryRunDiscord && !config.FlagSet(setFlags, "dry-run-discord") {
		dryRunDiscord = cfg.Worker.DryRunDiscord
	}
	if cfg.Worker.SendPendingOnly && !config.FlagSet(setFlags, "send-pending-only") {
		sendPendingOnly = cfg.Worker.SendPendingOnly
	}
	if cfg.Worker.PollInterval != "" && !config.FlagSet(setFlags, "poll-interval") {
		pollInterval = cfg.Worker.PollInterval
	}

	ttl, err := time.ParseDuration(leaseTTL)
	if err != nil {
		log.Fatalf("invalid --lease-ttl: %v", err)
	}
	pollEvery, err := time.ParseDuration(pollInterval)
	if err != nil {
		log.Fatalf("invalid --poll-interval: %v", err)
	}
	ctx := context.Background()
	pool, err := notifier.NewPool(ctx, dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	token := ""
	if sendDiscord && discordChannelID == "" {
		log.Printf("Discord sending enabled but --discord-channel-id/DISCORD_CHANNEL_ID is unset; notifications will remain pending")
	}
	if sendDiscord && !dryRunDiscord {
		token = os.Getenv(discordTokenEnv)
		if token == "" {
			log.Printf("Discord sending enabled but %s is unset; pending notifications will remain pending", discordTokenEnv)
		}
	}
	opts := notifier.WorkerOptions{
		MaxConcurrency:      maxConcurrency,
		LeaseTTL:            ttl,
		MaxAttempts:         maxAttempts,
		Limit:               limit,
		Once:                once,
		ClassifierCommand:   classifierCommand,
		Model:               model,
		DestinationRef:      discordChannelID,
		DiscordToken:        token,
		SendDiscord:         sendDiscord,
		DryRunDiscord:       dryRunDiscord,
		PollInterval:        pollEvery,
		NotifyTopicsAny:     cfg.Worker.NotifyTopicsAny,
		NotifyInterestNot:   cfg.Worker.NotifyInterestNot,
		NotifyConfidenceMin: cfg.Worker.NotifyConfidenceMin,
	}
	if sendPendingOnly {
		sent, err := notifier.SendPendingDiscord(ctx, pool, opts)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "sent=%d\n", sent)
		return
	}
	stats, err := notifier.RunWorker(ctx, pool, opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "claimed=%d succeeded=%d failed=%d notifications=%d sent=%d\n", stats.Claimed, stats.Succeeded, stats.Failed, stats.Notifications, stats.Sent)
}

func seenFlags() map[string]bool {
	flags := map[string]bool{}
	flag.Visit(func(f *flag.Flag) {
		flags[f.Name] = true
	})
	return flags
}
