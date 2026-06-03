package reposhellcli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	localconfig "github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/reposhell"
)

type fileConfig struct {
	blockConfig

	Reposhell  *blockConfig           `json:"reposhell"`
	Classifier localconfig.Classifier `json:"classifier"`
}

type blockConfig struct {
	localconfig.Reposhell

	DefaultRepo  string   `json:"default_repo"`
	VisibleRepos []string `json:"visible_repos"`
}

func LoadConfig(path string, opts Options) (RuntimeConfig, error) {
	opts = withDefaults(opts)
	expandedPath, err := reposhell.ExpandHome(path)
	if err != nil {
		return RuntimeConfig{}, err
	}
	file, err := os.Open(expandedPath)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("read config %s: %w", expandedPath, err)
	}
	defer func() { _ = file.Close() }()
	var raw fileConfig
	if err := json.NewDecoder(file).Decode(&raw); err != nil {
		return RuntimeConfig{}, fmt.Errorf("parse config %s: %w", expandedPath, err)
	}
	block := raw.blockConfig
	if raw.Reposhell != nil {
		block = *raw.Reposhell
		if block.DefaultRepo == "" {
			block.DefaultRepo = raw.Classifier.ReposhellDefaultRepo
		}
		if len(block.VisibleRepos) == 0 {
			block.VisibleRepos = raw.Classifier.ReposhellVisibleRepos
		}
	}
	return convertConfig(block, opts)
}

func convertConfig(cfg blockConfig, opts Options) (RuntimeConfig, error) {
	refreshInterval := reposhell.DefaultRefreshInterval
	if cfg.RefreshInterval != "" {
		parsed, err := time.ParseDuration(cfg.RefreshInterval)
		if err != nil {
			return RuntimeConfig{}, fmt.Errorf("reposhell.refresh_interval: %w", err)
		}
		refreshInterval = parsed
	}
	manager := reposhell.Config{
		Enabled:         cfg.Enabled,
		Root:            cfg.Root,
		MaxOutputBytes:  cfg.MaxOutputBytes,
		RefreshInterval: refreshInterval,
	}
	if manager.Root == "" {
		manager.Root = opts.DefaultRoot
	}
	if cfg.CommandTimeout != "" {
		parsed, err := time.ParseDuration(cfg.CommandTimeout)
		if err != nil {
			return RuntimeConfig{}, fmt.Errorf("reposhell.command_timeout: %w", err)
		}
		manager.CommandTimeout = parsed
	}
	for _, repo := range cfg.Repos {
		converted := reposhell.Repo{
			ID:         repo.ID,
			Remote:     repo.Remote,
			DefaultRef: repo.DefaultRef,
		}
		if repo.RefreshInterval != "" {
			parsed, err := time.ParseDuration(repo.RefreshInterval)
			if err != nil {
				return RuntimeConfig{}, fmt.Errorf("reposhell.repos[%s].refresh_interval: %w", repo.ID, err)
			}
			converted.RefreshInterval = parsed
		}
		manager.Repos = append(manager.Repos, converted)
	}
	out := RuntimeConfig{
		ManagerConfig: manager,
		Socket:        cfg.Socket,
		DefaultRepo:   cfg.DefaultRepo,
		VisibleRepos:  cfg.VisibleRepos,
	}
	if out.Socket == "" {
		out.Socket = opts.DefaultSocket
	}
	return out, nil
}
