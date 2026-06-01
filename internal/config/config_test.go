package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"repo":"example/repo","classifier":{"schema":"schema.json","prompt_template":"prompt.md","topic_taxonomy":"topics.json"},"worker":{"send_discord":true,"notify_topics_any":["local_models"]}}`), 0o644); err != nil {
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
