package app

import "github.com/osolmaz/localpager/internal/config"

const defaultReposhellSocket = "~/.local/state/localpager/reposhell.sock"

func ReposhellSocket(cfg config.Config) string {
	return valueOrDefault(cfg.Reposhell.Socket, defaultReposhellSocket)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
