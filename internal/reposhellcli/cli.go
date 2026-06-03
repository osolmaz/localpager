package reposhellcli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/reposhell"
)

const (
	StandaloneDefaultConfig = "~/.config/reposhell/config.json"
	LocalpagerDefaultConfig = "~/.config/localpager/config.json"
	StandaloneDefaultRoot   = "~/.local/state/reposhell"
	StandaloneDefaultSocket = "~/.local/state/reposhell/reposhell.sock"
)

type Options struct {
	UsagePrefix       string
	DefaultConfigPath string
	DefaultRoot       string
	DefaultSocket     string
}

type RuntimeConfig struct {
	ManagerConfig reposhell.Config
	Socket        string
	DefaultRepo   string
	VisibleRepos  []string
}

func StandaloneOptions() Options {
	return Options{
		UsagePrefix:       "reposhell",
		DefaultConfigPath: StandaloneDefaultConfig,
		DefaultRoot:       StandaloneDefaultRoot,
		DefaultSocket:     StandaloneDefaultSocket,
	}
}

func LocalpagerOptions() Options {
	return Options{
		UsagePrefix:       "localpager reposhell",
		DefaultConfigPath: LocalpagerDefaultConfig,
		DefaultRoot:       reposhell.DefaultRoot,
		DefaultSocket:     reposhell.DefaultSocket,
	}
}

func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, opts Options) int {
	opts = withDefaults(opts)
	if len(args) < 1 {
		usage(stderr, opts)
		return 2
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr, opts)
	case "exec":
		return runExec(args[1:], stdout, stderr, opts)
	case "shell":
		return runShell(args[1:], stdin, stdout, stderr, opts)
	case "status":
		return runStatus(args[1:], stdout, stderr, opts)
	default:
		usage(stderr, opts)
		return 2
	}
}

func runServe(args []string, stdout, stderr io.Writer, opts Options) int {
	fs := newFlagSet(opts.UsagePrefix+" serve", stderr)
	configPath := fs.String("config", opts.DefaultConfigPath, "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := LoadConfig(*configPath, opts)
	if err != nil {
		return fatal(stderr, err)
	}
	socketPath := cfg.Socket
	if *socket != "" {
		socketPath = *socket
	}
	fmt.Fprintf(stdout, "reposhell_socket=%s\n", socketPath)
	manager := reposhell.NewManager(cfg.ManagerConfig)
	if err := reposhell.NewServer(manager).ServeUnix(context.Background(), socketPath); err != nil {
		return fatal(stderr, err)
	}
	return 0
}

func runExec(args []string, stdout, stderr io.Writer, opts Options) int {
	fs := newFlagSet(opts.UsagePrefix+" exec", stderr)
	configPath := fs.String("config", opts.DefaultConfigPath, "JSON config file path")
	defaultRepo := fs.String("repo", "", "default configured repo id")
	command := fs.String("command", "", "read-only bash-shaped command")
	var visible []string
	fs.Func("visible-repo", "repo id visible to this run; repeatable", func(value string) error {
		visible = append(visible, value)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *command == "" {
		return fatal(stderr, fmt.Errorf("--command is required"))
	}
	cfg, err := LoadConfig(*configPath, opts)
	if err != nil {
		return fatal(stderr, err)
	}
	repoID, visibleRepos := repoSelection(cfg, *defaultRepo, visible)
	manager := reposhell.NewManager(cfg.ManagerConfig)
	binding, err := manager.Bind(context.Background(), repoID, visibleRepos)
	if err != nil {
		return fatal(stderr, err)
	}
	result := manager.Exec(context.Background(), reposhell.ExecRequest{Command: *command, Binding: binding})
	writeResult(stdout, stderr, result)
	return result.ExitCode
}

func runShell(args []string, stdin io.Reader, stdout, stderr io.Writer, opts Options) int {
	fs := newFlagSet(opts.UsagePrefix+" shell", stderr)
	configPath := fs.String("config", opts.DefaultConfigPath, "JSON config file path")
	defaultRepo := fs.String("repo", "", "default configured repo id")
	var visible []string
	fs.Func("visible-repo", "repo id visible to this run; repeatable", func(value string) error {
		visible = append(visible, value)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := LoadConfig(*configPath, opts)
	if err != nil {
		return fatal(stderr, err)
	}
	repoID, visibleRepos := repoSelection(cfg, *defaultRepo, visible)
	manager := reposhell.NewManager(cfg.ManagerConfig)
	binding, err := manager.Bind(context.Background(), repoID, visibleRepos)
	if err != nil {
		return fatal(stderr, err)
	}
	fmt.Fprintf(stdout, "reposhell bound cwd=%s repos=%s\n", binding.CWD, strings.Join(binding.VisibleRepos, ","))
	fmt.Fprintln(stdout, "type help for allowed commands; exit or quit to leave")
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	for {
		fmt.Fprintf(stdout, "reposhell %s> ", binding.CWD)
		if !scanner.Scan() {
			fmt.Fprintln(stdout)
			break
		}
		line := strings.TrimSpace(scanner.Text())
		switch line {
		case "":
			continue
		case "exit", "quit":
			return 0
		case "help":
			printShellHelp(stdout)
			continue
		}
		result := manager.Exec(context.Background(), reposhell.ExecRequest{Command: line, Binding: binding})
		writeResult(stdout, stderr, result)
		if result.ExitCode != 0 {
			fmt.Fprintf(stderr, "exit_code=%d\n", result.ExitCode)
		}
	}
	if err := scanner.Err(); err != nil {
		return fatal(stderr, err)
	}
	return 0
}

func runStatus(args []string, stdout, stderr io.Writer, opts Options) int {
	fs := newFlagSet(opts.UsagePrefix+" status", stderr)
	configPath := fs.String("config", opts.DefaultConfigPath, "JSON config file path")
	socket := fs.String("socket", "", "Unix socket path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	socketPath := *socket
	if socketPath == "" {
		cfg, err := LoadConfig(*configPath, opts)
		if err != nil {
			return fatal(stderr, err)
		}
		socketPath = cfg.Socket
	}
	body, err := request(socketPath, http.MethodGet, "/status", nil)
	if err != nil {
		return fatal(stderr, err)
	}
	_, _ = fmt.Fprint(stdout, body)
	return 0
}

func printShellHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "allowed: pwd, ls, find, rg, grep, sed -n, cat, head, tail, wc -l, git status --short, git show --name-only, git grep, git ls-files")
	fmt.Fprintln(stdout, "search: rg -n -i \"lm studio\" or grep -R -n -i \"lm studio\" .")
	fmt.Fprintln(stdout, "files: rg --files -g \"*.ts\" or git ls-files src")
	fmt.Fprintln(stdout, "examples: rg -n reposhell README.md | sed is not allowed; use one simple command at a time")
}

func writeResult(stdout, stderr io.Writer, result reposhell.ExecResult) {
	if result.Stdout != "" {
		_, _ = fmt.Fprint(stdout, result.Stdout)
	}
	if result.Stderr != "" {
		_, _ = fmt.Fprint(stderr, result.Stderr)
	}
}

func repoSelection(cfg RuntimeConfig, repo string, visible []string) (string, []string) {
	repoID := cfg.DefaultRepo
	if repo != "" {
		repoID = repo
	}
	visibleRepos := visible
	if len(visibleRepos) == 0 {
		visibleRepos = cfg.VisibleRepos
	}
	return repoID, visibleRepos
}

func request(socketPath, method, path string, payload any) (string, error) {
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
				expanded, err := reposhell.ExpandHome(socketPath)
				if err != nil {
					return nil, err
				}
				return net.Dial("unix", expanded)
			},
		},
	}
	req, err := http.NewRequest(method, "http://reposhell"+path, body)
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

func withDefaults(opts Options) Options {
	if opts.UsagePrefix == "" {
		opts.UsagePrefix = "reposhell"
	}
	if opts.DefaultConfigPath == "" {
		opts.DefaultConfigPath = StandaloneDefaultConfig
	}
	if opts.DefaultRoot == "" {
		opts.DefaultRoot = StandaloneDefaultRoot
	}
	if opts.DefaultSocket == "" {
		opts.DefaultSocket = StandaloneDefaultSocket
	}
	return opts
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func usage(stderr io.Writer, opts Options) {
	fmt.Fprintf(stderr, "usage: %s <serve|exec|shell|status> [flags]\n", opts.UsagePrefix)
}

func fatal(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, err)
	return 1
}
