package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"repo":"example/repo","classifier":{"schema":"schema.json","prompt_template":"prompt.md","topic_taxonomy":"topics.json","reposhell_default_repo":"example","reposhell_visible_repos":["example"],"context":{"github":{"include_body":true,"include_diff":false,"max_body_chars":1200}}},"reposhell":{"enabled":true,"root":"~/.local/state/localpager/reposhell","socket":"~/.local/state/localpager/reposhell.sock","command_timeout":"2s","refresh_interval":"24h","max_output_bytes":65536,"snapshot_retain":7,"repos":[{"id":"example","remote":"https://github.com/example/repo.git","default_ref":"origin/main","refresh_interval":"24h"}]},"worker":{"send_discord":true,"notify_topics_any":["local_models"],"agent_base_url":"http://127.0.0.1:1234/v1","agent_context_window":8192,"agent_max_tokens":768,"agent_temperature":0,"agent_top_p":1,"agent_seed":1234,"agent_presence_penalty":0,"agent_frequency_penalty":0,"agent_timeout_ms":5000,"model_unavailable_retry_delay":"5m"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "example/repo" {
		t.Fatalf("Repo = %q, want example/repo", cfg.Repo)
	}
	if len(cfg.Worker.NotifyTopicsAny) != 1 || cfg.Worker.NotifyTopicsAny[0] != "local_models" {
		t.Fatalf("NotifyTopicsAny = %v", cfg.Worker.NotifyTopicsAny)
	}
	if cfg.Classifier.Schema != "schema.json" {
		t.Fatalf("Classifier.Schema = %q, want schema.json", cfg.Classifier.Schema)
	}
	if cfg.Classifier.PromptTemplate != "prompt.md" {
		t.Fatalf("Classifier.PromptTemplate = %q, want prompt.md", cfg.Classifier.PromptTemplate)
	}
	if cfg.Classifier.TopicTaxonomy != "topics.json" {
		t.Fatalf("Classifier.TopicTaxonomy = %q, want topics.json", cfg.Classifier.TopicTaxonomy)
	}
	if cfg.Classifier.ReposhellDefaultRepo != "example" {
		t.Fatalf("Classifier.ReposhellDefaultRepo = %q, want example", cfg.Classifier.ReposhellDefaultRepo)
	}
	if cfg.Reposhell.Socket != "~/.local/state/localpager/reposhell.sock" {
		t.Fatalf("Reposhell.Socket = %q", cfg.Reposhell.Socket)
	}
	if cfg.Reposhell.SnapshotRetain != 7 {
		t.Fatalf("Reposhell.SnapshotRetain = %d, want 7", cfg.Reposhell.SnapshotRetain)
	}
	if len(cfg.Reposhell.Repos) != 1 || cfg.Reposhell.Repos[0].RefreshInterval != "24h" {
		t.Fatalf("Reposhell.Repos = %#v", cfg.Reposhell.Repos)
	}
	if cfg.Classifier.Context.GitHub.IncludeBody == nil || !*cfg.Classifier.Context.GitHub.IncludeBody {
		t.Fatalf("Classifier.Context.GitHub.IncludeBody = %v, want true", cfg.Classifier.Context.GitHub.IncludeBody)
	}
	if cfg.Classifier.Context.GitHub.IncludeDiff == nil || *cfg.Classifier.Context.GitHub.IncludeDiff {
		t.Fatalf("Classifier.Context.GitHub.IncludeDiff = %v, want false", cfg.Classifier.Context.GitHub.IncludeDiff)
	}
	if cfg.Classifier.Context.GitHub.MaxBodyChars != 1200 {
		t.Fatalf("Classifier.Context.GitHub.MaxBodyChars = %d, want 1200", cfg.Classifier.Context.GitHub.MaxBodyChars)
	}
	if cfg.Worker.AgentBaseURL != "http://127.0.0.1:1234/v1" {
		t.Fatalf("Worker.AgentBaseURL = %q, want local base URL", cfg.Worker.AgentBaseURL)
	}
	if cfg.Worker.AgentContextWindow != 8192 {
		t.Fatalf("Worker.AgentContextWindow = %d, want 8192", cfg.Worker.AgentContextWindow)
	}
	if cfg.Worker.AgentMaxTokens != 768 {
		t.Fatalf("Worker.AgentMaxTokens = %d, want 768", cfg.Worker.AgentMaxTokens)
	}
	if cfg.Worker.AgentTemperature == nil || *cfg.Worker.AgentTemperature != 0 {
		t.Fatalf("Worker.AgentTemperature = %v, want 0", cfg.Worker.AgentTemperature)
	}
	if cfg.Worker.AgentTopP == nil || *cfg.Worker.AgentTopP != 1 {
		t.Fatalf("Worker.AgentTopP = %v, want 1", cfg.Worker.AgentTopP)
	}
	if cfg.Worker.AgentSeed == nil || *cfg.Worker.AgentSeed != 1234 {
		t.Fatalf("Worker.AgentSeed = %v, want 1234", cfg.Worker.AgentSeed)
	}
	if cfg.Worker.AgentPresencePenalty == nil || *cfg.Worker.AgentPresencePenalty != 0 {
		t.Fatalf("Worker.AgentPresencePenalty = %v, want 0", cfg.Worker.AgentPresencePenalty)
	}
	if cfg.Worker.AgentFrequencyPenalty == nil || *cfg.Worker.AgentFrequencyPenalty != 0 {
		t.Fatalf("Worker.AgentFrequencyPenalty = %v, want 0", cfg.Worker.AgentFrequencyPenalty)
	}
	if cfg.Worker.AgentTimeoutMS != 5000 {
		t.Fatalf("Worker.AgentTimeoutMS = %d, want 5000", cfg.Worker.AgentTimeoutMS)
	}
	if cfg.Worker.ModelUnavailableRetryDelay != "5m" {
		t.Fatalf("Worker.ModelUnavailableRetryDelay = %q, want 5m", cfg.Worker.ModelUnavailableRetryDelay)
	}
}

func TestLoadEmptyPathReturnsZeroConfig(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Repo != "" {
		t.Fatalf("Repo = %q, want empty", cfg.Repo)
	}
}

func TestLoadRejectsBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"repo":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("Load() err = nil, want parse error")
	}
}

func TestLoadReturnsReadError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatalf("Load() err = nil, want read error")
	}
}

func TestFlagSet(t *testing.T) {
	flags := map[string]bool{"repo": true}
	if !FlagSet(flags, "repo") {
		t.Fatalf("FlagSet(repo) = false, want true")
	}
	if FlagSet(flags, "missing") {
		t.Fatalf("FlagSet(missing) = true, want false")
	}
}

func TestValidateWarnsWhenDiscordHasNoTopicGate(t *testing.T) {
	cfg := Config{
		Repo: "example/repo",
		Worker: Worker{
			SendDiscord: true,
		},
	}
	warnings := cfg.Validate()
	if len(warnings) == 0 {
		t.Fatalf("Validate() returned no warnings")
	}
}

func TestValidateWarnsWhenRepoIsPlaceholder(t *testing.T) {
	warnings := Config{Repo: "owner/repo"}.Validate()
	if len(warnings) == 0 {
		t.Fatal("Validate() returned no warnings for placeholder repo")
	}
}

func TestValidateWarnsOnUnsupportedWatchSource(t *testing.T) {
	cfg := Config{Repo: "example/repo", Watch: Watch{Sources: []string{"gitcrawl", "unknown"}}}
	warnings := cfg.Validate()
	if len(warnings) != 1 {
		t.Fatalf("Validate() warnings = %v, want one unsupported source warning", warnings)
	}
}

func TestValidateAcceptsConfiguredTopicGate(t *testing.T) {
	cfg := Config{
		Repo: "example/repo",
		Worker: Worker{
			SendDiscord:     true,
			NotifyTopicsAny: []string{"local_models"},
		},
	}
	warnings := cfg.Validate()
	if len(warnings) != 0 {
		t.Fatalf("Validate() warnings = %v, want none", warnings)
	}
}
