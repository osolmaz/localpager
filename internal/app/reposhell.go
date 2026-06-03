package app

import (
	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/reposhell"
)

func ReposhellSocket(cfg config.Config) string {
	return valueOrDefault(cfg.Reposhell.Socket, reposhell.DefaultSocket)
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
