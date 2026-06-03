package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/app"
	"github.com/osolmaz/localpager/internal/reposhell"
)

func runReposhell(args []string) {
	if len(args) < 1 {
		reposhellUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "serve":
		runReposhellServe(args[1:])
	case "exec":
		runReposhellExec(args[1:])
	case "status":
		runReposhellStatus(args[1:])
	default:
		reposhellUsage()
		os.Exit(2)
	}
}

func runReposhellServe(args []string) {
	fs := flag.NewFlagSet("reposhell serve", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	readerConfig, err := app.ReposhellConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	socketPath := valueOrDefault(*socket, app.ReposhellSocket(cfg))
	app.Printf(os.Stdout, "reposhell_socket=%s\n", socketPath)
	manager := reposhell.NewManager(readerConfig)
	if err := reposhell.NewServer(manager).ServeUnix(context.Background(), socketPath); err != nil {
		log.Fatal(err)
	}
}

func runReposhellExec(args []string) {
	fs := flag.NewFlagSet("reposhell exec", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	defaultRepo := fs.String("repo", "", "default configured repo id")
	command := fs.String("command", "", "read-only bash-shaped command")
	var visible app.MultiFlag
	fs.Var(&visible, "visible-repo", "repo id visible to this run; repeatable")
	_ = fs.Parse(args)
	if *command == "" {
		log.Fatal("--command is required")
	}
	cfg := loadConfig(*configPath)
	readerConfig, err := app.ReposhellConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	repoID := valueOrDefault(*defaultRepo, cfg.Classifier.ReposhellDefaultRepo)
	visibleRepos := []string(visible)
	if len(visibleRepos) == 0 {
		visibleRepos = cfg.Classifier.ReposhellVisibleRepos
	}
	manager := reposhell.NewManager(readerConfig)
	binding, err := manager.Bind(context.Background(), repoID, visibleRepos)
	if err != nil {
		log.Fatal(err)
	}
	result := manager.Exec(context.Background(), reposhell.ExecRequest{Command: *command, Binding: binding})
	if result.Stdout != "" {
		app.Printf(os.Stdout, "%s", result.Stdout)
	}
	if result.Stderr != "" {
		app.Printf(os.Stderr, "%s", result.Stderr)
	}
	os.Exit(result.ExitCode)
}

func runReposhellStatus(args []string) {
	fs := flag.NewFlagSet("reposhell status", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	socketPath := valueOrDefault(*socket, app.ReposhellSocket(cfg))
	body, err := reposhellRequest(socketPath, http.MethodGet, "/status", nil)
	if err != nil {
		log.Fatal(err)
	}
	app.Printf(os.Stdout, "%s", body)
}

func reposhellRequest(socketPath, method, path string, payload any) (string, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		body = bytes.NewReader(raw)
	}
	client := http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.Dial("unix", mustExpand(socketPath))
			},
		},
	}
	req, err := http.NewRequest(method, "http://localpager"+path, body)
	if err != nil {
		return "", err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return string(raw), nil
}

func reposhellUsage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: localpager reposhell <serve|exec|status> [flags]")
}
