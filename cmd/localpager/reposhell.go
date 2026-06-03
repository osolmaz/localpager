package main

import (
	"fmt"
	"os"
	"time"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/reposhell"
	"github.com/osolmaz/reposhell/reposhellcli"
)

const (
	localpagerReposhellDefaultConfig = "~/.config/localpager/config.json"
	localpagerReposhellDefaultRoot   = "~/.local/state/localpager/reposhell"
	localpagerReposhellDefaultSocket = "~/.local/state/localpager/reposhell.sock"
)

func runReposhell(args []string) {
	os.Exit(reposhellcli.Run(args, os.Stdin, os.Stdout, os.Stderr, localpagerReposhellOptions()))
}

func localpagerReposhellOptions() reposhellcli.Options {
	return reposhellcli.Options{
		UsagePrefix:       "localpager reposhell",
		DefaultConfigPath: localpagerReposhellDefaultConfig,
		DefaultRoot:       localpagerReposhellDefaultRoot,
		DefaultSocket:     localpagerReposhellDefaultSocket,
		LoadConfig:        loadLocalpagerReposhellConfig,
	}
}

func loadLocalpagerReposhellConfig(
	path string,
	opts reposhellcli.Options,
) (reposhellcli.RuntimeConfig, error) {
	expandedPath, err := reposhell.ExpandHome(path)
	if err != nil {
		return reposhellcli.RuntimeConfig{}, err
	}
	cfg, err := config.Load(expandedPath)
	if err != nil {
		return reposhellcli.RuntimeConfig{}, err
	}
	return localpagerReposhellRuntimeConfig(cfg, opts)
}

func localpagerReposhellRuntimeConfig(
	cfg config.Config,
	opts reposhellcli.Options,
) (reposhellcli.RuntimeConfig, error) {
	manager := reposhell.Config{
		Enabled:        cfg.Reposhell.Enabled,
		Root:           cfg.Reposhell.Root,
		MaxOutputBytes: cfg.Reposhell.MaxOutputBytes,
		SnapshotRetain: cfg.Reposhell.SnapshotRetain,
	}
	if manager.Root == "" {
		manager.Root = opts.DefaultRoot
	}
	if cfg.Reposhell.RefreshInterval != "" {
		parsed, err := time.ParseDuration(cfg.Reposhell.RefreshInterval)
		if err != nil {
			return reposhellcli.RuntimeConfig{}, fmt.Errorf("reposhell.refresh_interval: %w", err)
		}
		manager.RefreshInterval = parsed
	}
	if cfg.Reposhell.CommandTimeout != "" {
		parsed, err := time.ParseDuration(cfg.Reposhell.CommandTimeout)
		if err != nil {
			return reposhellcli.RuntimeConfig{}, fmt.Errorf("reposhell.command_timeout: %w", err)
		}
		manager.CommandTimeout = parsed
	}
	for _, repo := range cfg.Reposhell.Repos {
		converted := reposhell.Repo{
			ID:         repo.ID,
			Remote:     repo.Remote,
			DefaultRef: repo.DefaultRef,
		}
		if repo.RefreshInterval != "" {
			parsed, err := time.ParseDuration(repo.RefreshInterval)
			if err != nil {
				return reposhellcli.RuntimeConfig{}, fmt.Errorf("reposhell.repos[%s].refresh_interval: %w", repo.ID, err)
			}
			converted.RefreshInterval = parsed
		}
		manager.Repos = append(manager.Repos, converted)
	}
	socket := cfg.Reposhell.Socket
	if socket == "" {
		socket = opts.DefaultSocket
	}
	return reposhellcli.RuntimeConfig{
		ManagerConfig: manager,
		Socket:        socket,
		DefaultRepo:   cfg.Classifier.ReposhellDefaultRepo,
		VisibleRepos:  cfg.Classifier.ReposhellVisibleRepos,
	}, nil
}
