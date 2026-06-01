package config

import "testing"

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
