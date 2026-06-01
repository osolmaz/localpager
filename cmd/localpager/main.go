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

	"github.com/osolmaz/localpager/internal/app"
	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/localpager"
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
		app.Println(os.Stdout, "config_ok=true")
		return
	}
	for _, warning := range warnings {
		app.Printf(os.Stdout, "warning=%s\n", warning)
	}
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "JSON config file path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	ctx := context.Background()
	pool, err := localpager.NewPool(ctx, valueOrDefault(cfg.DBPath, localpager.DefaultDBPath))
	if err != nil {
		log.Fatal(err)
	}
	defer app.ClosePool(pool)

	app.Printf(os.Stdout, "repo=%s\n", cfg.Repo)
	app.Printf(os.Stdout, "db=%s\n", valueOrDefault(cfg.DBPath, localpager.DefaultDBPath))
	app.Printf(os.Stdout, "model=%s\n", cfg.Worker.Model)
	app.Printf(os.Stdout, "classifier_schema=%s\n", cfg.Classifier.Schema)
	app.Printf(os.Stdout, "classifier_prompt_template=%s\n", cfg.Classifier.PromptTemplate)
	app.Printf(os.Stdout, "classifier_topic_taxonomy=%s\n", cfg.Classifier.TopicTaxonomy)
	app.Printf(os.Stdout, "send_discord=%t\n", cfg.Worker.SendDiscord)
	app.Printf(os.Stdout, "dry_run_discord=%t\n", cfg.Worker.DryRunDiscord)
	app.Printf(os.Stdout, "notify_topics_any=%s\n", strings.Join(cfg.Worker.NotifyTopicsAny, ","))
	printCounts(ctx, pool, "jobs", &localpager.Job{}, "status")
	printCounts(ctx, pool, "notifications", &localpager.Notification{}, "status")
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
	id, err := localpager.SendDiscordMessage(context.Background(), token, channelID, *message)
	if err != nil {
		log.Fatal(err)
	}
	app.Printf(os.Stdout, "sent=true id=%s\n", id)
}

func runInstallService(args []string) {
	fs := flag.NewFlagSet("install-service", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	binDir := fs.String("bin-dir", "~/.local/bin", "directory containing localpager binaries")
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
	servicePath := defaultServicePath()
	sharedUnit := commandUnitOptions{
		configPath:  expandedConfig,
		workDir:     expandedWorkDir,
		envFile:     expandedEnv,
		secretsFile: expandedSecrets,
		servicePath: servicePath,
	}

	units := map[string]string{
		"localpager-worker.service": commandUnit(sharedUnit.withCommand(
			"Localpager worker", "simple", filepath.Join(expandedBinDir, "localpager-worker"), "Restart=always\nRestartSec=10s\n",
		)),
		"localpager-watch.service": commandUnit(sharedUnit.withCommand(
			"Localpager source watcher", "simple", filepath.Join(expandedBinDir, "localpager-watch"), "Restart=always\nRestartSec=10s\n",
		)),
		"localpager-enqueue-github.service": commandUnit(sharedUnit.withCommand(
			"Localpager GitHub enqueue", "oneshot", filepath.Join(expandedBinDir, "localpager-enqueue-github"), "TimeoutStartSec=10m\n",
		)),
		"localpager-enqueue-github.timer": timerUnit("Run Localpager GitHub enqueue every 10 minutes", "localpager-enqueue-github.service"),
	}
	for name, body := range units {
		path := filepath.Join(expandedSystemdDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			log.Fatal(err)
		}
		app.Printf(os.Stdout, "wrote=%s\n", path)
	}
}

func loadConfig(path string) config.Config {
	cfg, err := config.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	return cfg
}

func printCounts(ctx context.Context, pool *localpager.Pool, name string, model any, column string) {
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	if err := pool.GORM().WithContext(ctx).Model(model).Select(column + " AS status, count(*) AS count").Group(column).Order(column).Scan(&rows).Error; err != nil {
		log.Fatal(err)
	}
	for _, row := range rows {
		app.Printf(os.Stdout, "%s_%s=%d\n", name, row.Status, row.Count)
	}
}

func printLast(ctx context.Context, pool *localpager.Pool) {
	var item localpager.Item
	if err := pool.GORM().WithContext(ctx).Order("last_seen_at DESC").First(&item).Error; err == nil {
		app.Printf(os.Stdout, "last_item=%s %s\n", item.SourceKind, item.SourceRef)
	}
	var result localpager.Result
	if err := pool.GORM().WithContext(ctx).Order("created_at DESC").First(&result).Error; err == nil {
		app.Printf(os.Stdout, "last_result=%s\n", result.CreatedAt.Format(time.RFC3339))
	}
	var notification localpager.Notification
	if err := pool.GORM().WithContext(ctx).Order("updated_at DESC").First(&notification).Error; err == nil {
		app.Printf(os.Stdout, "last_notification=%s %s\n", notification.Status, notification.UpdatedAt.Format(time.RFC3339))
	}
}

type commandUnitOptions struct {
	description string
	serviceType string
	binaryPath  string
	configPath  string
	workDir     string
	envFile     string
	secretsFile string
	servicePath string
	extra       string
}

func (opts commandUnitOptions) withCommand(description, serviceType, binaryPath, extra string) commandUnitOptions {
	opts.description = description
	opts.serviceType = serviceType
	opts.binaryPath = binaryPath
	opts.extra = extra
	return opts
}

func commandUnit(opts commandUnitOptions) string {
	return fmt.Sprintf(`[Unit]
Description=%s
After=network-online.target
Wants=network-online.target

[Service]
Type=%s
Environment=HOME=%%h
Environment=PATH=%s
EnvironmentFile=-%s
EnvironmentFile=-%s
WorkingDirectory=%s
ExecStart=%s --config %s
%s
[Install]
WantedBy=default.target
`, opts.description, opts.serviceType, opts.servicePath, opts.envFile, opts.secretsFile, opts.workDir, opts.binaryPath, opts.configPath, opts.extra)
}

func timerUnit(description, unit string) string {
	return fmt.Sprintf(`[Unit]
Description=%s

[Timer]
OnBootSec=2m
OnUnitActiveSec=10m
AccuracySec=1m
RandomizedDelaySec=1m
Persistent=true
Unit=%s

[Install]
WantedBy=timers.target
`, description, unit)
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultServicePath() string {
	pathValue := strings.TrimSpace(os.Getenv("PATH"))
	if pathValue == "" {
		return "%h/.local/bin:/usr/local/bin:/usr/bin:/bin"
	}
	parts := strings.Split(pathValue, ":")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || strings.Contains(part, ".codex/tmp") || strings.Contains(part, "node_modules/@openai/codex") {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == 0 {
		return "%h/.local/bin:/usr/local/bin:/usr/bin:/bin"
	}
	return strings.Join(kept, ":")
}

func mustExpand(path string) string {
	expanded, err := localpager.ExpandPath(path)
	if err != nil {
		log.Fatal(err)
	}
	return expanded
}

func usage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: localpager <validate|status|test-discord|install-service> [flags]")
}
