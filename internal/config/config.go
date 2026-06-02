package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Repo             string     `json:"repo"`
	DBPath           string     `json:"db"`
	GitcrawlDBPath   string     `json:"gitcrawl_db"`
	GitHubBaseURL    string     `json:"github_base_url"`
	GitHubTokenEnv   string     `json:"github_token_env"`
	SourceType       string     `json:"source_type"`
	ProcessorName    string     `json:"processor_name"`
	ProcessorVersion string     `json:"processor_version"`
	RecentWindow     string     `json:"recent_window"`
	CutoverAt        string     `json:"cutover_at"`
	Enqueue          Enqueue    `json:"enqueue"`
	Watch            Watch      `json:"watch"`
	Classifier       Classifier `json:"classifier"`
	Worker           Worker     `json:"worker"`
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

type Classifier struct {
	Schema         string  `json:"schema"`
	PromptTemplate string  `json:"prompt_template"`
	TopicTaxonomy  string  `json:"topic_taxonomy"`
	Context        Context `json:"context"`
}

type Context struct {
	GitHub GitHubContext `json:"github"`
}

type GitHubContext struct {
	IncludeBody          *bool `json:"include_body"`
	IncludeLabels        *bool `json:"include_labels"`
	IncludeComments      *bool `json:"include_comments"`
	IncludeChangedFiles  *bool `json:"include_changed_files"`
	IncludeDiff          *bool `json:"include_diff"`
	MaxBodyChars         int   `json:"max_body_chars"`
	MaxCommentsChars     int   `json:"max_comments_chars"`
	MaxChangedFilesChars int   `json:"max_changed_files_chars"`
	MaxDiffChars         int   `json:"max_diff_chars"`
}

type Worker struct {
	MaxConcurrency             int      `json:"max_concurrency"`
	LeaseTTL                   string   `json:"lease_ttl"`
	MaxAttempts                int      `json:"max_attempts"`
	Limit                      int      `json:"limit"`
	Once                       bool     `json:"once"`
	ClassifierCommand          string   `json:"classifier_command"`
	Model                      string   `json:"model"`
	AgentBaseURL               string   `json:"agent_base_url"`
	AgentContextWindow         int      `json:"agent_context_window"`
	AgentMaxTokens             int      `json:"agent_max_tokens"`
	AgentTimeoutMS             int      `json:"agent_timeout_ms"`
	ModelUnavailableRetryDelay string   `json:"model_unavailable_retry_delay"`
	DiscordChannelID           string   `json:"discord_channel_id"`
	DiscordChannelIDEnv        string   `json:"discord_channel_id_env"`
	DiscordTokenEnv            string   `json:"discord_token_env"`
	SendDiscord                bool     `json:"send_discord"`
	DryRunDiscord              bool     `json:"dry_run_discord"`
	SendPendingOnly            bool     `json:"send_pending_only"`
	PollInterval               string   `json:"poll_interval"`
	NotifyTopicsAny            []string `json:"notify_topics_any"`
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

func (cfg Config) Validate() []string {
	var warnings []string
	warnings = appendWarning(warnings, validateRepo(cfg.Repo))
	warnings = appendWarning(warnings, validateDiscordTopics(cfg.Worker))
	return append(warnings, validateWatchSources(cfg.Watch.Sources)...)
}

func validateRepo(repo string) string {
	if strings.TrimSpace(repo) == "" || repo == "owner/repo" {
		return "repo is unset or still owner/repo"
	}
	return ""
}

func validateDiscordTopics(worker Worker) string {
	if worker.SendDiscord && !worker.DryRunDiscord && len(worker.NotifyTopicsAny) == 0 {
		return "send_discord is enabled without worker.notify_topics_any; all classifier results may notify"
	}
	return ""
}

func validateWatchSources(sources []string) []string {
	var warnings []string
	for _, source := range sources {
		switch strings.ToLower(strings.TrimSpace(source)) {
		case "", "gitcrawl", "github":
		default:
			warnings = append(warnings, fmt.Sprintf("unsupported watch source %q", source))
		}
	}
	return warnings
}

func appendWarning(warnings []string, warning string) []string {
	if warning == "" {
		return warnings
	}
	return append(warnings, warning)
}
