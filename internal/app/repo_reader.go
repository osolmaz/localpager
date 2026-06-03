package app

import (
	"fmt"
	"time"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/reporeader"
)

func RepoReaderConfig(cfg config.Config) (reporeader.Config, error) {
	reader := cfg.RepoReader
	out := reporeader.Config{
		Enabled:         reader.Enabled,
		Root:            valueOrDefault(reader.Root, reporeader.DefaultRoot),
		MaxOutputBytes:  reader.MaxOutputBytes,
		RefreshInterval: reporeader.DefaultRefreshInterval,
	}
	if reader.CommandTimeout != "" {
		duration, err := time.ParseDuration(reader.CommandTimeout)
		if err != nil {
			return out, fmt.Errorf("repo_reader.command_timeout: %w", err)
		}
		out.CommandTimeout = duration
	}
	if reader.RefreshInterval != "" {
		duration, err := time.ParseDuration(reader.RefreshInterval)
		if err != nil {
			return out, fmt.Errorf("repo_reader.refresh_interval: %w", err)
		}
		out.RefreshInterval = duration
	}
	for _, repo := range reader.Repos {
		converted := reporeader.Repo{
			ID:         repo.ID,
			Remote:     repo.Remote,
			DefaultRef: repo.DefaultRef,
		}
		if repo.RefreshInterval != "" {
			duration, err := time.ParseDuration(repo.RefreshInterval)
			if err != nil {
				return out, fmt.Errorf("repo_reader.repos[%s].refresh_interval: %w", repo.ID, err)
			}
			converted.RefreshInterval = duration
		}
		out.Repos = append(out.Repos, converted)
	}
	return out, nil
}

func RepoReaderSocket(cfg config.Config) string {
	return valueOrDefault(cfg.RepoReader.Socket, reporeader.DefaultSocket)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
