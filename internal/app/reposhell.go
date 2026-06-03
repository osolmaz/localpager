package app

import (
	"fmt"
	"time"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/reposhell"
)

func ReposhellConfig(cfg config.Config) (reposhell.Config, error) {
	reader := cfg.Reposhell
	out := reposhell.Config{
		Enabled:         reader.Enabled,
		Root:            valueOrDefault(reader.Root, reposhell.DefaultRoot),
		MaxOutputBytes:  reader.MaxOutputBytes,
		RefreshInterval: reposhell.DefaultRefreshInterval,
	}
	if reader.CommandTimeout != "" {
		duration, err := time.ParseDuration(reader.CommandTimeout)
		if err != nil {
			return out, fmt.Errorf("reposhell.command_timeout: %w", err)
		}
		out.CommandTimeout = duration
	}
	if reader.RefreshInterval != "" {
		duration, err := time.ParseDuration(reader.RefreshInterval)
		if err != nil {
			return out, fmt.Errorf("reposhell.refresh_interval: %w", err)
		}
		out.RefreshInterval = duration
	}
	for _, repo := range reader.Repos {
		converted := reposhell.Repo{
			ID:         repo.ID,
			Remote:     repo.Remote,
			DefaultRef: repo.DefaultRef,
		}
		if repo.RefreshInterval != "" {
			duration, err := time.ParseDuration(repo.RefreshInterval)
			if err != nil {
				return out, fmt.Errorf("reposhell.repos[%s].refresh_interval: %w", repo.ID, err)
			}
			converted.RefreshInterval = duration
		}
		out.Repos = append(out.Repos, converted)
	}
	return out, nil
}

func ReposhellSocket(cfg config.Config) string {
	return valueOrDefault(cfg.Reposhell.Socket, reposhell.DefaultSocket)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
