package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Repo             string  `json:"repo"`
	DBPath           string  `json:"db"`
	GitcrawlDBPath   string  `json:"gitcrawl_db"`
	SourceType       string  `json:"source_type"`
	ProcessorName    string  `json:"processor_name"`
	ProcessorVersion string  `json:"processor_version"`
	RecentWindow     string  `json:"recent_window"`
	CutoverAt        string  `json:"cutover_at"`
	Enqueue          Enqueue `json:"enqueue"`
	Watch            Watch   `json:"watch"`
	Worker           Worker  `json:"worker"`
}

type Enqueue struct {
	Limit            int  `json:"limit"`
	InitialHydration bool `json:"initial_hydration"`
}

type Watch struct {
	Sources  []string `json:"sources"`
	Interval string   `json:"interval"`
	Once     bool     `json:"once"`
	Limit    int      `json:"limit"`
}

type Worker struct {
	MaxConcurrency      int      `json:"max_concurrency"`
	LeaseTTL            string   `json:"lease_ttl"`
	MaxAttempts         int      `json:"max_attempts"`
	Limit               int      `json:"limit"`
	Once                bool     `json:"once"`
	ClassifierCommand   string   `json:"classifier_command"`
	Model               string   `json:"model"`
	DiscordChannelID    string   `json:"discord_channel_id"`
	DiscordChannelIDEnv string   `json:"discord_channel_id_env"`
	DiscordTokenEnv     string   `json:"discord_token_env"`
	SendDiscord         bool     `json:"send_discord"`
	DryRunDiscord       bool     `json:"dry_run_discord"`
	SendPendingOnly     bool     `json:"send_pending_only"`
	PollInterval        string   `json:"poll_interval"`
	NotifyTopicsAny     []string `json:"notify_topics_any"`
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func FlagSet(flags map[string]bool, name string) bool {
	return flags[name]
}
