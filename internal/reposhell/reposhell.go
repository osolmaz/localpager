package reposhell

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

const (
	DefaultRefreshInterval = 24 * time.Hour
	DefaultCommandTimeout  = 2 * time.Second
	DefaultMaxOutputBytes  = 65536
	DefaultRoot            = "~/.local/state/localpager/reposhell"
	DefaultSocket          = "~/.local/state/localpager/reposhell.sock"
)

var (
	ErrPolicy     = errors.New("reposhell policy denied command")
	snapshotLocks sync.Map
)

type Config struct {
	Enabled         bool
	Root            string
	Repos           []Repo
	CommandTimeout  time.Duration
	MaxOutputBytes  int64
	RefreshInterval time.Duration
}

type Repo struct {
	ID              string
	Remote          string
	DefaultRef      string
	RefreshInterval time.Duration
}

type Manager struct {
	Config Config
}

type Binding struct {
	DefaultRepo  string
	VisibleRepos []string
	CWD          string
	Snapshots    map[string]string
	Roots        map[string]string
	GitDirs      map[string]string
	IndexFiles   map[string]string
}

type ExecRequest struct {
	Command string
	Binding Binding
}

type ExecResult struct {
	Stdout      string `json:"stdout"`
	Stderr      string `json:"stderr"`
	ExitCode    int    `json:"exit_code"`
	PolicyError string `json:"policy_error,omitempty"`
	Truncated   bool   `json:"truncated"`
}

func NewManager(cfg Config) Manager {
	if cfg.CommandTimeout == 0 {
		cfg.CommandTimeout = DefaultCommandTimeout
	}
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultRefreshInterval
	}
	return Manager{Config: cfg}
}

func (m Manager) Bind(ctx context.Context, defaultRepo string, visibleRepos []string) (Binding, error) {
	if defaultRepo == "" {
		return Binding{}, fmt.Errorf("default repo is required")
	}
	if len(visibleRepos) == 0 {
		visibleRepos = []string{defaultRepo}
	}
	seen := map[string]bool{}
	roots := map[string]string{}
	gitDirs := map[string]string{}
	indexFiles := map[string]string{}
	snapshots := map[string]string{}
	for _, repoID := range visibleRepos {
		if seen[repoID] {
			continue
		}
		seen[repoID] = true
		repo, ok := m.repo(repoID)
		if !ok {
			return Binding{}, fmt.Errorf("repo %q is not configured", repoID)
		}
		snapshot, commit, gitDir, indexFile, err := m.EnsureSnapshot(ctx, repo)
		if err != nil {
			return Binding{}, err
		}
		roots[repoID] = snapshot
		gitDirs[repoID] = gitDir
		indexFiles[repoID] = indexFile
		snapshots[repoID] = commit
	}
	if _, ok := roots[defaultRepo]; !ok {
		return Binding{}, fmt.Errorf("default repo %q is not visible", defaultRepo)
	}
	return Binding{
		DefaultRepo:  defaultRepo,
		VisibleRepos: keysInOrder(visibleRepos, roots),
		CWD:          "/repo/" + defaultRepo,
		Snapshots:    snapshots,
		Roots:        roots,
		GitDirs:      gitDirs,
		IndexFiles:   indexFiles,
	}, nil
}

func (m Manager) EnsureSnapshot(ctx context.Context, repo Repo) (string, string, string, string, error) {
	if repo.ID == "" {
		return "", "", "", "", fmt.Errorf("repo id is required")
	}
	if repo.Remote == "" {
		return "", "", "", "", fmt.Errorf("repo %s remote is required", repo.ID)
	}
	ref := repo.DefaultRef
	if ref == "" {
		ref = "origin/main"
	}
	root, err := expandHome(m.Config.Root)
	if err != nil {
		return "", "", "", "", err
	}
	if root == "" {
		root = filepath.Join(os.TempDir(), "localpager-reposhell")
	}
	mirror := filepath.Join(root, "mirrors", safeID(repo.ID)+".git")
	lock := snapshotLock(filepath.Clean(mirror))
	lock.Lock()
	defer lock.Unlock()
	if err := m.ensureMirror(ctx, repo, mirror); err != nil {
		return "", "", "", "", err
	}
	commit, err := resolveCommit(ctx, mirror, repo.ID, ref)
	if err != nil {
		return "", "", "", "", err
	}
	snapshot := filepath.Join(root, "snapshots", safeID(repo.ID), commit)
	indexFile := filepath.Join(root, "snapshots", safeID(repo.ID), commit+".index")
	marker := filepath.Join(root, "snapshots", safeID(repo.ID), commit+".ready")
	if err := ensureSnapshotCheckout(ctx, repo.ID, mirror, snapshot, indexFile, marker, commit); err != nil {
		return "", "", "", "", err
	}
	return snapshot, commit, mirror, indexFile, nil
}

func (m Manager) ensureMirror(ctx context.Context, repo Repo, mirror string) error {
	if _, err := os.Stat(mirror); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(mirror), 0o755); err != nil {
			return fmt.Errorf("create mirror dir: %w", err)
		}
		if result := runGit(ctx, "", "clone", "--mirror", repo.Remote, mirror); result.err != nil {
			return fmt.Errorf("clone mirror for %s: %w stderr=%s", repo.ID, result.err, strings.TrimSpace(result.stderr))
		}
	} else if err != nil {
		return fmt.Errorf("stat mirror: %w", err)
	} else if shouldRefresh(mirror, repo.RefreshInterval, m.Config.RefreshInterval) {
		if result := runGit(ctx, "", "--git-dir", mirror, "fetch", "--prune"); result.err != nil {
			return fmt.Errorf("fetch mirror for %s: %w stderr=%s", repo.ID, result.err, strings.TrimSpace(result.stderr))
		}
	}
	return nil
}

func resolveCommit(ctx context.Context, mirror, repoID, ref string) (string, error) {
	rev := resolveRef(ctx, mirror, ref)
	if rev.err != nil {
		return "", fmt.Errorf("resolve %s %s: %w stderr=%s", repoID, ref, rev.err, strings.TrimSpace(rev.stderr))
	}
	commit := strings.TrimSpace(rev.stdout)
	if !commitRE.MatchString(commit) {
		return "", fmt.Errorf("resolve %s %s: invalid commit %q", repoID, ref, commit)
	}
	return commit, nil
}

func ensureSnapshotCheckout(ctx context.Context, repoID, mirror, snapshot, indexFile, marker, commit string) error {
	if fileExists(marker) && fileExists(indexFile) {
		return nil
	}
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	checkout := runGitWithEnv(ctx, "", []string{"GIT_INDEX_FILE=" + indexFile}, "--git-dir", mirror, "--work-tree", snapshot, "checkout", "-f", commit)
	if checkout.err != nil {
		return fmt.Errorf("checkout snapshot for %s: %w stderr=%s", repoID, checkout.err, strings.TrimSpace(checkout.stderr))
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return fmt.Errorf("create snapshot marker dir: %w", err)
	}
	if err := os.WriteFile(marker, []byte(commit+"\n"), 0o444); err != nil {
		return fmt.Errorf("write snapshot marker: %w", err)
	}
	return nil
}

func (m Manager) Exec(ctx context.Context, req ExecRequest) ExecResult {
	timeout := m.Config.CommandTimeout
	if timeout == 0 {
		timeout = DefaultCommandTimeout
	}
	maxOutput := m.Config.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutputBytes
	}
	argv, err := parseCommand(req.Command)
	if err != nil {
		return policyResult(err)
	}
	plan, err := buildExecPlan(argv, req.Binding)
	if err != nil {
		return policyResult(err)
	}
	if plan.SyntheticStdout != "" {
		return ExecResult{Stdout: plan.SyntheticStdout, ExitCode: 0}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, plan.Name, plan.Args...)
	cmd.Dir = plan.Dir
	cmd.Env = append([]string{"PATH=/usr/bin:/bin", "LC_ALL=C", "LANG=C"}, plan.Env...)
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	var stdout cappedBuffer
	var stderr cappedBuffer
	stdout.limit = maxOutput
	stderr.limit = maxOutput
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result := ExecResult{
		Stdout:    virtualizeOutput(stdout.String(), req.Binding),
		Stderr:    virtualizeOutput(stderr.String(), req.Binding),
		ExitCode:  exitCode(err),
		Truncated: stdout.truncated || stderr.truncated,
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.Stderr = appendLine(result.Stderr, "reposhell: command timed out")
		result.ExitCode = 124
	}
	return result
}

type execPlan struct {
	Name            string
	Args            []string
	Dir             string
	Env             []string
	SyntheticStdout string
}

func buildExecPlan(argv []string, binding Binding) (execPlan, error) {
	if len(argv) == 0 {
		return execPlan{}, deny("empty command")
	}
	root, ok := binding.Roots[binding.DefaultRepo]
	if !ok || root == "" {
		return execPlan{}, deny("default repo is not bound")
	}
	switch argv[0] {
	case "pwd":
		if len(argv) != 1 {
			return execPlan{}, deny("pwd does not accept arguments")
		}
		return execPlan{SyntheticStdout: binding.CWD + "\n", Dir: root}, nil
	case "ls":
		return planPathCommand("/bin/ls", argv[1:], binding, lsArg)
	case "cat":
		return planPathCommand("/bin/cat", argv[1:], binding, noFlags)
	case "head":
		return planHead(argv[1:], binding)
	case "sed":
		return planSed(argv[1:], binding)
	case "find":
		return planFind(argv[1:], binding)
	case "rg":
		return planSearch("rg", argv[1:], binding)
	case "grep":
		return planSearch("/bin/grep", argv[1:], binding)
	case "git":
		return planGit(argv[1:], binding)
	default:
		return execPlan{}, deny("unsupported command %q", argv[0])
	}
}

func parseCommand(command string) ([]string, error) {
	if strings.TrimSpace(command) == "" {
		return nil, deny("empty command")
	}
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "reposhell-command")
	if err != nil {
		return nil, deny("parse command: %v", err)
	}
	if len(file.Stmts) != 1 || len(file.Last) > 0 {
		return nil, deny("exactly one simple command is allowed")
	}
	stmt := file.Stmts[0]
	if stmt.Negated || stmt.Background || stmt.Coprocess || len(stmt.Redirs) > 0 || stmt.Semicolon.IsValid() {
		return nil, deny("shell control operators and redirects are not allowed")
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, deny("only simple commands are allowed")
	}
	if len(call.Assigns) > 0 {
		return nil, deny("environment assignments are not allowed")
	}
	argv := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		value, err := literalWord(word)
		if err != nil {
			return nil, err
		}
		if strings.ContainsRune(value, '\x00') {
			return nil, deny("nul bytes are not allowed")
		}
		argv = append(argv, value)
	}
	return argv, nil
}

func literalWord(word *syntax.Word) (string, error) {
	var b strings.Builder
	for _, part := range word.Parts {
		value, err := literalPart(part)
		if err != nil {
			return "", err
		}
		b.WriteString(value)
	}
	return b.String(), nil
}

func literalPart(part syntax.WordPart) (string, error) {
	switch node := part.(type) {
	case *syntax.Lit:
		return node.Value, nil
	case *syntax.SglQuoted:
		if node.Dollar {
			return "", deny("dollar single quotes are not allowed")
		}
		return node.Value, nil
	case *syntax.DblQuoted:
		if node.Dollar {
			return "", deny("dollar double quotes are not allowed")
		}
		var b strings.Builder
		for _, inner := range node.Parts {
			value, err := literalPart(inner)
			if err != nil {
				return "", err
			}
			b.WriteString(value)
		}
		return b.String(), nil
	default:
		return "", deny("shell expansions are not allowed")
	}
}

type argValidator func(arg string) (isFlag bool, err error)

func noFlags(arg string) (bool, error) {
	if strings.HasPrefix(arg, "-") {
		return true, deny("flags are not allowed")
	}
	return false, nil
}

func lsArg(arg string) (bool, error) {
	switch arg {
	case "-a", "-l", "-la", "-al", "-1":
		return true, nil
	}
	if strings.HasPrefix(arg, "-") {
		return true, deny("unsupported ls flag %q", arg)
	}
	return false, nil
}

func planPathCommand(name string, args []string, binding Binding, validator argValidator) (execPlan, error) {
	root := binding.Roots[binding.DefaultRepo]
	out := make([]string, 0, len(args))
	for _, arg := range args {
		isFlag, err := validator(arg)
		if err != nil {
			return execPlan{}, err
		}
		if isFlag {
			out = append(out, arg)
			continue
		}
		resolved, err := resolveVirtualPath(binding, arg)
		if err != nil {
			return execPlan{}, err
		}
		out = append(out, resolved)
	}
	if len(out) == 0 && name == "/bin/ls" {
		out = append(out, root)
	}
	return execPlan{Name: name, Args: out, Dir: root}, nil
}

func planHead(args []string, binding Binding) (execPlan, error) {
	if len(args) < 1 {
		return execPlan{}, deny("head requires a path")
	}
	out := []string{}
	index := 0
	if args[0] == "-n" {
		if len(args) < 3 {
			return execPlan{}, deny("head -n requires a count and path")
		}
		if err := validatePositiveBounded(args[1], 1000); err != nil {
			return execPlan{}, err
		}
		out = append(out, "-n", args[1])
		index = 2
	} else if strings.HasPrefix(args[0], "-n") && len(args[0]) > 2 {
		count := strings.TrimPrefix(args[0], "-n")
		if err := validatePositiveBounded(count, 1000); err != nil {
			return execPlan{}, err
		}
		out = append(out, args[0])
		index = 1
	}
	if index >= len(args) {
		return execPlan{}, deny("head requires a path")
	}
	for _, pathArg := range args[index:] {
		resolved, err := resolveVirtualPath(binding, pathArg)
		if err != nil {
			return execPlan{}, err
		}
		out = append(out, resolved)
	}
	return execPlan{Name: "/usr/bin/head", Args: out, Dir: binding.Roots[binding.DefaultRepo]}, nil
}

func planSed(args []string, binding Binding) (execPlan, error) {
	if len(args) != 3 || args[0] != "-n" {
		return execPlan{}, deny("sed only allows: sed -n START,ENDp path")
	}
	if err := validateSedRange(args[1]); err != nil {
		return execPlan{}, err
	}
	resolved, err := resolveVirtualPath(binding, args[2])
	if err != nil {
		return execPlan{}, err
	}
	return execPlan{Name: "/bin/sed", Args: []string{"-n", args[1], resolved}, Dir: binding.Roots[binding.DefaultRepo]}, nil
}

func planFind(args []string, binding Binding) (execPlan, error) {
	out := []string{}
	index := 0
	for index < len(args) && !strings.HasPrefix(args[index], "-") {
		resolved, err := resolveVirtualPath(binding, args[index])
		if err != nil {
			return execPlan{}, err
		}
		out = append(out, resolved)
		index++
	}
	if len(out) == 0 {
		out = append(out, binding.Roots[binding.DefaultRepo])
	}
	for index < len(args) {
		next, err := appendFindPredicate(&out, args, index)
		if err != nil {
			return execPlan{}, err
		}
		index = next
	}
	return execPlan{Name: "/usr/bin/find", Args: out, Dir: binding.Roots[binding.DefaultRepo]}, nil
}

func appendFindPredicate(out *[]string, args []string, index int) (int, error) {
	arg := args[index]
	switch arg {
	case "-maxdepth":
		value, err := findValue(args, index, arg)
		if err != nil {
			return index, err
		}
		if err := validatePositiveBounded(value, 10); err != nil {
			return index, err
		}
		*out = append(*out, arg, value)
	case "-type":
		value, err := findValue(args, index, arg)
		if err != nil {
			return index, err
		}
		if value != "f" && value != "d" {
			return index, deny("find -type only allows f or d")
		}
		*out = append(*out, arg, value)
	case "-name", "-iname":
		value, err := findValue(args, index, arg)
		if err != nil {
			return index, err
		}
		if strings.Contains(value, "/") {
			return index, deny("find name patterns cannot contain slashes")
		}
		*out = append(*out, arg, value)
	default:
		return index, deny("unsupported find argument %q", arg)
	}
	return index + 2, nil
}

func findValue(args []string, index int, flag string) (string, error) {
	if index+1 >= len(args) {
		return "", deny("find %s requires a value", flag)
	}
	return args[index+1], nil
}

func planSearch(name string, args []string, binding Binding) (execPlan, error) {
	if len(args) == 0 {
		return execPlan{}, deny("%s requires a pattern", filepath.Base(name))
	}
	out := []string{}
	index := 0
	for index < len(args) {
		arg := args[index]
		if arg == "--" {
			out = append(out, "--")
			index++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		consumed, err := appendSearchFlag(&out, args[index:])
		if err != nil {
			return execPlan{}, err
		}
		index += consumed
	}
	if index >= len(args) {
		return execPlan{}, deny("%s requires a pattern", filepath.Base(name))
	}
	out = append(out, args[index])
	index++
	if index == len(args) {
		out = append(out, binding.Roots[binding.DefaultRepo])
	} else {
		for _, pathArg := range args[index:] {
			resolved, err := resolveVirtualPath(binding, pathArg)
			if err != nil {
				return execPlan{}, err
			}
			out = append(out, resolved)
		}
	}
	return execPlan{Name: name, Args: out, Dir: binding.Roots[binding.DefaultRepo]}, nil
}

func appendSearchFlag(out *[]string, args []string) (int, error) {
	arg := args[0]
	switch arg {
	case "-n", "--line-number", "-i", "--ignore-case", "-S", "--smart-case", "-F", "--fixed-strings", "-w", "--word-regexp", "-l", "--files-with-matches":
		*out = append(*out, arg)
		return 1, nil
	case "-C", "-A", "-B", "--context", "--after-context", "--before-context":
		if len(args) < 2 {
			return 0, deny("%s requires a count", arg)
		}
		if err := validatePositiveBounded(args[1], 50); err != nil {
			return 0, err
		}
		*out = append(*out, arg, args[1])
		return 2, nil
	default:
		for _, prefix := range []string{"-C", "-A", "-B", "--context=", "--after-context=", "--before-context="} {
			if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
				if err := validatePositiveBounded(strings.TrimPrefix(arg, prefix), 50); err != nil {
					return 0, err
				}
				*out = append(*out, arg)
				return 1, nil
			}
		}
		return 0, deny("unsupported search flag %q", arg)
	}
}

func planGit(args []string, binding Binding) (execPlan, error) {
	if len(args) == 0 {
		return execPlan{}, deny("git requires a subcommand")
	}
	root := binding.Roots[binding.DefaultRepo]
	gitDir := binding.GitDirs[binding.DefaultRepo]
	if gitDir == "" {
		return execPlan{}, deny("default repo git dir is not bound")
	}
	gitEnv, err := boundGitEnv(binding)
	if err != nil {
		return execPlan{}, err
	}
	switch args[0] {
	case "status":
		if len(args) == 2 && args[1] == "--short" {
			return execPlan{Name: "/usr/bin/git", Args: appendGitWorkTreeArgs(gitDir, root, args...), Dir: root, Env: gitEnv}, nil
		}
	case "show":
		if len(args) == 2 && args[1] == "--name-only" {
			commit, err := boundCommit(binding)
			if err != nil {
				return execPlan{}, err
			}
			showArgs := []string{"show", "--name-only", commit}
			return execPlan{Name: "/usr/bin/git", Args: appendGitWorkTreeArgs(gitDir, root, showArgs...), Dir: root, Env: gitEnv}, nil
		}
	case "grep":
		if len(args) >= 2 {
			return planGitGrep(args, binding)
		}
	}
	return execPlan{}, deny("unsupported git command")
}

func boundCommit(binding Binding) (string, error) {
	commit := binding.Snapshots[binding.DefaultRepo]
	if commit == "" {
		return "", deny("default repo snapshot is not bound")
	}
	return commit, nil
}

func boundGitEnv(binding Binding) ([]string, error) {
	indexFile := binding.IndexFiles[binding.DefaultRepo]
	if indexFile == "" {
		return nil, deny("default repo git index is not bound")
	}
	return []string{"GIT_INDEX_FILE=" + indexFile}, nil
}

func planGitGrep(args []string, binding Binding) (execPlan, error) {
	out := []string{"grep"}
	index := 1
	for index < len(args) && strings.HasPrefix(args[index], "-") {
		switch args[index] {
		case "-n", "-i", "-F", "-w", "-l":
			out = append(out, args[index])
			index++
		default:
			return execPlan{}, deny("unsupported git grep flag %q", args[index])
		}
	}
	if index >= len(args) {
		return execPlan{}, deny("git grep requires a pattern")
	}
	out = append(out, args[index])
	index++
	if index < len(args) {
		out = append(out, "--")
	}
	for _, pathArg := range args[index:] {
		virtual, err := cleanRepoRelativePath(binding, pathArg)
		if err != nil {
			return execPlan{}, err
		}
		out = append(out, virtual)
	}
	gitEnv, err := boundGitEnv(binding)
	if err != nil {
		return execPlan{}, err
	}
	return execPlan{Name: "/usr/bin/git", Args: appendGitWorkTreeArgs(binding.GitDirs[binding.DefaultRepo], binding.Roots[binding.DefaultRepo], out...), Dir: binding.Roots[binding.DefaultRepo], Env: gitEnv}, nil
}

func resolveVirtualPath(binding Binding, raw string) (string, error) {
	if raw == "" {
		return "", deny("empty path is not allowed")
	}
	repoID := binding.DefaultRepo
	rel := raw
	if strings.HasPrefix(raw, "/repo/") {
		rest := strings.TrimPrefix(raw, "/repo/")
		parts := strings.SplitN(rest, "/", 2)
		repoID = parts[0]
		if len(parts) == 1 {
			rel = "."
		} else {
			rel = parts[1]
		}
	} else if filepath.IsAbs(raw) {
		return "", deny("absolute paths outside /repo are not allowed")
	}
	root, ok := binding.Roots[repoID]
	if !ok {
		return "", deny("repo %q is not visible", repoID)
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", deny("path escapes repo root")
	}
	joined := filepath.Join(root, clean)
	resolved, err := resolveExistingOrParent(joined)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repo root: %w", err)
	}
	if !insideRoot(rootReal, resolved) {
		return "", deny("path escapes repo root")
	}
	return joined, nil
}

func cleanRepoRelativePath(binding Binding, raw string) (string, error) {
	resolved, err := resolveVirtualPath(binding, raw)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(binding.Roots[binding.DefaultRepo], resolved)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(rel, "..") {
		return "", deny("git grep paths must be in default repo")
	}
	return filepath.ToSlash(rel), nil
}

func resolveExistingOrParent(path string) (string, error) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved, nil
	}
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return filepath.Clean(path), nil
		}
		return "", fmt.Errorf("resolve path parent: %w", err)
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func insideRoot(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func validatePositiveBounded(raw string, max int) error {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > max {
		return deny("count must be between 0 and %d", max)
	}
	return nil
}

func validateSedRange(raw string) error {
	match := sedRangeRE.FindStringSubmatch(raw)
	if match == nil {
		return deny("sed range must be START,ENDp")
	}
	start, _ := strconv.Atoi(match[1])
	end, _ := strconv.Atoi(match[2])
	if start < 1 || end < start || end-start > 1000 {
		return deny("sed range must be positive and at most 1000 lines")
	}
	return nil
}

func shouldRefresh(mirror string, repoInterval, defaultInterval time.Duration) bool {
	interval := repoInterval
	if interval == 0 {
		interval = defaultInterval
	}
	if interval == 0 {
		interval = DefaultRefreshInterval
	}
	info, err := os.Stat(mirror)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) >= interval
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runGit(ctx context.Context, dir string, args ...string) gitResult {
	return runGitWithEnv(ctx, dir, nil, args...)
}

func runGitWithEnv(ctx context.Context, dir string, env []string, args ...string) gitResult {
	cmd := exec.CommandContext(ctx, "/usr/bin/git", args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return gitResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func resolveRef(ctx context.Context, mirror, ref string) gitResult {
	rev := runGit(ctx, "", "--git-dir", mirror, "rev-parse", ref)
	if rev.err == nil || !strings.HasPrefix(ref, "origin/") {
		return rev
	}
	return runGit(ctx, "", "--git-dir", mirror, "rev-parse", strings.TrimPrefix(ref, "origin/"))
}

func appendGitWorkTreeArgs(gitDir, workTree string, args ...string) []string {
	out := []string{"--git-dir", gitDir, "--work-tree", workTree}
	return append(out, args...)
}

type gitResult struct {
	stdout string
	stderr string
	err    error
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func policyResult(err error) ExecResult {
	return ExecResult{ExitCode: 2, PolicyError: err.Error(), Stderr: err.Error() + "\n"}
}

func deny(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPolicy, fmt.Sprintf(format, args...))
}

func appendLine(body, line string) string {
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + line + "\n"
}

func virtualizeOutput(body string, binding Binding) string {
	for repoID, root := range binding.Roots {
		if root == "" {
			continue
		}
		virtual := "/repo/" + repoID
		body = strings.ReplaceAll(body, root, virtual)
		body = strings.ReplaceAll(body, filepath.ToSlash(root), virtual)
	}
	return body
}

func safeID(id string) string {
	return hex.EncodeToString([]byte(id))
}

func keysInOrder(ids []string, roots map[string]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if roots[id] == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func snapshotLock(key string) *sync.Mutex {
	value, _ := snapshotLocks.LoadOrStore(key, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (m Manager) repo(id string) (Repo, bool) {
	for _, repo := range m.Config.Repos {
		if repo.ID == id {
			return repo, true
		}
	}
	return Repo{}, false
}

func expandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if path == "~" {
		return home, nil
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return "", fmt.Errorf("unsupported home path %q", path)
}

func ExpandHome(path string) (string, error) {
	return expandHome(path)
}

type cappedBuffer struct {
	buf       bytes.Buffer
	limit     int64
	written   int64
	truncated bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.written += int64(len(p))
	if b.limit <= 0 {
		return len(p), nil
	}
	remaining := b.limit - int64(b.buf.Len())
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	chunk := p
	if int64(len(chunk)) > remaining {
		chunk = chunk[:remaining]
		b.truncated = true
	}
	_, _ = b.buf.Write(chunk)
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	if !b.truncated {
		return b.buf.String()
	}
	out := b.buf.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + "[reposhell: output truncated]\n"
}

var (
	commitRE             = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sedRangeRE           = regexp.MustCompile(`^([0-9]+),([0-9]+)p$`)
	_          io.Writer = (*cappedBuffer)(nil)
)
