package app

import (
	"log"
	"time"

	"github.com/osolmaz/localpager/internal/config"
)

func LogConfigWarnings(configPath string, cfg config.Config) {
	if configPath == "" {
		return
	}
	for _, warning := range cfg.Validate() {
		log.Printf("config warning: %s", warning)
	}
}

func ParseDurationFlag(name, value string) time.Duration {
	duration, err := time.ParseDuration(value)
	if err != nil {
		log.Fatalf("invalid --%s: %v", name, err)
	}
	return duration
}

func ParseCutoverFlag(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		log.Fatalf("invalid --cutover-at: %v", err)
	}
	return &parsed
}
