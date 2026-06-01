package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

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
		MaxConcurrency:    maxConcurrency,
		LeaseTTL:          ttl,
		MaxAttempts:       maxAttempts,
		Limit:             limit,
		Once:              once,
		ClassifierCommand: classifierCommand,
		Model:             model,
		DestinationRef:    discordChannelID,
		DiscordToken:      token,
		SendDiscord:       sendDiscord,
		DryRunDiscord:     dryRunDiscord,
		PollInterval:      pollEvery,
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
