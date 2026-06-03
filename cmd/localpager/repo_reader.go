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
	"github.com/osolmaz/localpager/internal/reporeader"
)

func runRepoReader(args []string) {
	if len(args) < 1 {
		repoReaderUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "serve":
		runRepoReaderServe(args[1:])
	case "exec":
		runRepoReaderExec(args[1:])
	case "status":
		runRepoReaderStatus(args[1:])
	default:
		repoReaderUsage()
		os.Exit(2)
	}
}

func runRepoReaderServe(args []string) {
	fs := flag.NewFlagSet("repo-reader serve", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	readerConfig, err := app.RepoReaderConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	socketPath := valueOrDefault(*socket, app.RepoReaderSocket(cfg))
	app.Printf(os.Stdout, "repo_reader_socket=%s\n", socketPath)
	manager := reporeader.NewManager(readerConfig)
	if err := reporeader.NewServer(manager).ServeUnix(context.Background(), socketPath); err != nil {
		log.Fatal(err)
	}
}

func runRepoReaderExec(args []string) {
	fs := flag.NewFlagSet("repo-reader exec", flag.ExitOnError)
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
	readerConfig, err := app.RepoReaderConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}
	repoID := valueOrDefault(*defaultRepo, cfg.Classifier.RepoReaderDefaultRepo)
	visibleRepos := []string(visible)
	if len(visibleRepos) == 0 {
		visibleRepos = cfg.Classifier.RepoReaderVisibleRepos
	}
	manager := reporeader.NewManager(readerConfig)
	binding, err := manager.Bind(context.Background(), repoID, visibleRepos)
	if err != nil {
		log.Fatal(err)
	}
	result := manager.Exec(context.Background(), reporeader.ExecRequest{Command: *command, Binding: binding})
	if result.Stdout != "" {
		app.Printf(os.Stdout, "%s", result.Stdout)
	}
	if result.Stderr != "" {
		app.Printf(os.Stderr, "%s", result.Stderr)
	}
	os.Exit(result.ExitCode)
}

func runRepoReaderStatus(args []string) {
	fs := flag.NewFlagSet("repo-reader status", flag.ExitOnError)
	configPath := fs.String("config", "~/.config/localpager/config.json", "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	_ = fs.Parse(args)
	cfg := loadConfig(*configPath)
	socketPath := valueOrDefault(*socket, app.RepoReaderSocket(cfg))
	body, err := repoReaderRequest(socketPath, http.MethodGet, "/status", nil)
	if err != nil {
		log.Fatal(err)
	}
	app.Printf(os.Stdout, "%s", body)
}

func repoReaderRequest(socketPath, method, path string, payload any) (string, error) {
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

func repoReaderUsage() {
	_, _ = fmt.Fprintln(os.Stderr, "usage: localpager repo-reader <serve|exec|status> [flags]")
}
