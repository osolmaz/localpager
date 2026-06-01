package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/notifier"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		runValidate(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "test-discord":
		runTestDiscord(os.Args[2:])
	case "install-service":
		runInstallService(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runValidate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	configPath := fs.String("config", "", "JSON config file path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		fmt.Println("config_ok=true")
		return
	}
	for _, warning := range warnings {
		fmt.Fprintf(os.Stdout, "warning=%s\n", warning)
	}
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "JSON config file path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	ctx := context.Background()
	pool, err := notifier.NewPool(ctx, valueOrDefault(cfg.DBPath, notifier.DefaultDBPath))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	fmt.Fprintf(os.Stdout, "repo=%s\n", cfg.Repo)
	fmt.Fprintf(os.Stdout, "db=%s\n", valueOrDefault(cfg.DBPath, notifier.DefaultDBPath))
	fmt.Fprintf(os.Stdout, "model=%s\n", cfg.Worker.Model)
	fmt.Fprintf(os.Stdout, "send_discord=%t\n", cfg.Worker.SendDiscord)
	fmt.Fprintf(os.Stdout, "dry_run_discord=%t\n", cfg.Worker.DryRunDiscord)
	fmt.Fprintf(os.Stdout, "notify_topics_any=%s\n", strings.Join(cfg.Worker.NotifyTopicsAny, ","))
	printCounts(ctx, pool, "jobs", &notifier.Job{}, "status")
	printCounts(ctx, pool, "notifications", &notifier.Notification{}, "status")
	printLast(ctx, pool)
}

func runTestDiscord(args []string) {
	fs := flag.NewFlagSet("test-discord", flag.ExitOnError)
	configPath := fs.String("config", "", "JSON config file path")
	message := fs.String("message", "Localpager Discord test", "message to send")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	channelID := cfg.Worker.DiscordChannelID
	if channelID == "" && cfg.Worker.DiscordChannelIDEnv != "" {
		channelID = os.Getenv(cfg.Worker.DiscordChannelIDEnv)
	}
	tokenEnv := valueOrDefault(cfg.Worker.DiscordTokenEnv, "DISCORD_BOT_TOKEN")
	token := os.Getenv(tokenEnv)
	if channelID == "" {
		log.Fatal("discord channel id is unset")
	}
	if token == "" {
		log.Fatalf("%s is unset", tokenEnv)
	}
	id, err := notifier.SendDiscordMessage(context.Background(), token, channelID, *message)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "sent=true id=%s\n", id)
}

func runInstallService(args []string) {
	fs := flag.NewFlagSet("install-service", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	binDir := fs.String("bin-dir", "~/.local/bin", "directory containing notifier binaries")
	systemdDir := fs.String("systemd-dir", "~/.config/systemd/user", "systemd user unit directory")
	workDir := fs.String("work-dir", ".", "working directory for relative classifier commands")
	envFile := fs.String("env-file", "~/.config/localpager/localpager.env", "optional EnvironmentFile")
	secretsFile := fs.String("secrets-file", "~/.config/localpager/secrets.env", "optional EnvironmentFile")
	_ = fs.Parse(args)

	expandedSystemdDir := mustExpand(*systemdDir)
	if err := os.MkdirAll(expandedSystemdDir, 0o755); err != nil {
		log.Fatal(err)
	}
	expandedBinDir := mustExpand(*binDir)
	expandedConfig := mustExpand(*configPath)
	expandedWorkDir, err := filepath.Abs(*workDir)
	if err != nil {
		log.Fatal(err)
	}
	expandedEnv := mustExpand(*envFile)
	expandedSecrets := mustExpand(*secretsFile)

	units := map[string]string{
		"localpager-notifier-worker.service": serviceUnit("Localpager notifier worker", filepath.Join(expandedBinDir, "notifier-worker"), expandedConfig, expandedWorkDir, expandedEnv, expandedSecrets),
		"localpager-notifier-watch.service":  serviceUnit("Localpager source watcher", filepath.Join(expandedBinDir, "notifier-watch"), expandedConfig, expandedWorkDir, expandedEnv, expandedSecrets),
	}
	for name, body := range units {
		path := filepath.Join(expandedSystemdDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stdout, "wrote=%s\n", path)
	}
}

func loadConfig(path string) config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

func printCounts(ctx context.Context, pool *notifier.Pool, name string, model any, column string) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := pool.GORM().WithContext(ctx).Model(model).Select(column + " AS status, count(*) AS count").Group(column).Order(column).Scan(&rows).Error; err != nil {
		log.Fatal(err)
	}
	for _, row := range rows {
		fmt.Fprintf(os.Stdout, "%s_%s=%d\n", name, row.Status, row.Count)
	}
}

func printLast(ctx context.Context, pool *notifier.Pool) {
	var item notifier.Item
	if err := pool.GORM().WithContext(ctx).Order("last_seen_at DESC").First(&item).Error; err == nil {
		fmt.Fprintf(os.Stdout, "last_item=%s %s\n", item.SourceKind, item.SourceRef)
	}
	var result notifier.Result
	if err := pool.GORM().WithContext(ctx).Order("created_at DESC").First(&result).Error; err == nil {
		fmt.Fprintf(os.Stdout, "last_result=%s\n", result.CreatedAt.Format(time.RFC3339))
	}
	var notification notifier.Notification
	if err := pool.GORM().WithContext(ctx).Order("updated_at DESC").First(&notification).Error; err == nil {
		fmt.Fprintf(os.Stdout, "last_notification=%s %s\n", notification.Status, notification.UpdatedAt.Format(time.RFC3339))
	}
}

func serviceUnit(description, binaryPath, configPath, workDir, envFile, secretsFile string) string {
	return fmt.Sprintf(`[Unit]
Description=%s
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=HOME=%%h
Environment=PATH=%%h/.local/bin:/usr/local/bin:/usr/bin:/bin
EnvironmentFile=-%s
EnvironmentFile=-%s
WorkingDirectory=%s
ExecStart=%s --config %s
Restart=always
RestartSec=10s

[Install]
WantedBy=default.target
`, description, envFile, secretsFile, workDir, binaryPath, configPath)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func mustExpand(path string) string {
	expanded, err := notifier.ExpandPath(path)
	if err != nil {
		log.Fatal(err)
	}
	return expanded
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: localpager <validate|status|test-discord|install-service> [flags]")
}
