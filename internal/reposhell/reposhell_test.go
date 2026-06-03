package reposhell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecAllowsReadOnlyCommandsInVirtualRepo(t *testing.T) {
	ctx := context.Background()
	source := testGitRepo(t, map[string]string{
		"README.md":     "Localpager reposhell\nSecond line\n",
		"docs/note.txt": "tool_calling notes\n",
		"dash.txt":      "-n\n",
		"src/app.go":    "package app\n// tool_calling implementation\n",
		"src/app.ts":    "export const topic = 'tool_calling';\n",
	})
	manager := NewManager(Config{
		Root:            filepath.Join(t.TempDir(), "state"),
		RefreshInterval: time.Hour,
		Repos: []Repo{{
			ID:         "project",
			Remote:     source,
			DefaultRef: "origin/main",
		}},
	})
	binding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		command string
		want    string
	}{
		{"pwd", "/repo/project\n"},
		{"cat README.md", "Localpager reposhell"},
		{"head -n 1 README.md", "Localpager reposhell\n"},
		{"tail -n 1 README.md", "Second line\n"},
		{"wc -l README.md", "2 /repo/project/README.md\n"},
		{"sed -n 2,2p README.md", "Second line\n"},
		{"find . -maxdepth 2 -type f -name note.txt", "docs/note.txt"},
		{"rg --files -g '*.ts' src", "src/app.ts\n"},
		{"rg -n -g '*.go' tool_calling src", "src/app.go:2:// tool_calling implementation\n"},
		{"grep -n Local README.md", "1:Localpager reposhell\n"},
		{"grep -R -n tool_calling docs", "docs/note.txt:1:tool_calling notes\n"},
		{"grep -- -n dash.txt", "-n\n"},
		{"git ls-files src", "src/app.go\nsrc/app.ts\n"},
		{"git grep -n tool_calling docs", "docs/note.txt:1:tool_calling notes\n"},
		{"git show --name-only", "README.md"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			result := manager.Exec(ctx, ExecRequest{Command: tc.command, Binding: binding})
			if result.PolicyError != "" {
				t.Fatalf("PolicyError = %q", result.PolicyError)
			}
			if result.ExitCode != 0 {
				t.Fatalf("ExitCode = %d stderr=%s", result.ExitCode, result.Stderr)
			}
			if !strings.Contains(result.Stdout, tc.want) {
				t.Fatalf("Stdout = %q, want to contain %q", result.Stdout, tc.want)
			}
			if strings.Contains(result.Stdout, binding.Roots["project"]) {
				t.Fatalf("Stdout leaked snapshot root %q: %q", binding.Roots["project"], result.Stdout)
			}
		})
	}
}

func TestExecSupportsVisibleReposAndDefaultRepo(t *testing.T) {
	ctx := context.Background()
	alpha := testGitRepo(t, map[string]string{"name.txt": "alpha\n"})
	beta := testGitRepo(t, map[string]string{"name.txt": "beta\n"})
	manager := NewManager(Config{
		Root: filepath.Join(t.TempDir(), "state"),
		Repos: []Repo{
			{ID: "alpha", Remote: alpha, DefaultRef: "main"},
			{ID: "beta", Remote: beta, DefaultRef: "main"},
		},
	})
	binding, err := manager.Bind(ctx, "alpha", []string{"alpha", "beta"})
	if err != nil {
		t.Fatal(err)
	}

	if got := mustExec(t, manager, binding, "cat name.txt"); got != "alpha\n" {
		t.Fatalf("default repo read = %q, want alpha", got)
	}
	if got := mustExec(t, manager, binding, "cat /repo/beta/name.txt"); got != "beta\n" {
		t.Fatalf("visible repo read = %q, want beta", got)
	}
	result := manager.Exec(ctx, ExecRequest{Command: "cat /repo/gamma/name.txt", Binding: binding})
	if result.PolicyError == "" {
		t.Fatal("PolicyError is empty for invisible repo")
	}
}

func TestGitShowUsesBoundSnapshotCommit(t *testing.T) {
	ctx := context.Background()
	source := testGitRepo(t, map[string]string{"old.txt": "old\n"})
	manager := NewManager(Config{
		Root:  filepath.Join(t.TempDir(), "state"),
		Repos: []Repo{{ID: "project", Remote: source, DefaultRef: "main"}},
	})
	binding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "new.txt")
	git(t, source, "commit", "-m", "advance")
	git(t, "", "--git-dir", binding.GitDirs["project"], "fetch", "--prune")

	result := manager.Exec(ctx, ExecRequest{Command: "git show --name-only", Binding: binding})
	if result.PolicyError != "" || result.ExitCode != 0 {
		t.Fatalf("git show failed: policy=%q exit=%d stderr=%s", result.PolicyError, result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "old.txt") {
		t.Fatalf("Stdout = %q, want old snapshot file", result.Stdout)
	}
	if strings.Contains(result.Stdout, "new.txt") {
		t.Fatalf("Stdout = %q, should not include file from later commit", result.Stdout)
	}
}

func TestGitCommandsUseBoundSnapshotIndex(t *testing.T) {
	ctx := context.Background()
	source := testGitRepo(t, map[string]string{"old.txt": "old\n"})
	manager := NewManager(Config{
		Root:  filepath.Join(t.TempDir(), "state"),
		Repos: []Repo{{ID: "project", Remote: source, DefaultRef: "main"}},
	})
	oldBinding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "new.txt")
	git(t, source, "commit", "-m", "advance")
	if _, err := manager.Bind(ctx, "project", []string{"project"}); err != nil {
		t.Fatal(err)
	}

	oldGrep := manager.Exec(ctx, ExecRequest{Command: "git grep -n old", Binding: oldBinding})
	if oldGrep.PolicyError != "" || oldGrep.ExitCode != 0 {
		t.Fatalf("old git grep failed: policy=%q exit=%d stderr=%s", oldGrep.PolicyError, oldGrep.ExitCode, oldGrep.Stderr)
	}
	if !strings.Contains(oldGrep.Stdout, "old.txt:1:old") {
		t.Fatalf("old git grep stdout = %q", oldGrep.Stdout)
	}
	newGrep := manager.Exec(ctx, ExecRequest{Command: "git grep -n new", Binding: oldBinding})
	if strings.Contains(newGrep.Stdout, "new.txt") {
		t.Fatalf("old binding saw newer file: %q", newGrep.Stdout)
	}
	status := manager.Exec(ctx, ExecRequest{Command: "git status --short", Binding: oldBinding})
	if status.PolicyError != "" || status.ExitCode != 0 {
		t.Fatalf("git status failed: policy=%q exit=%d stderr=%s", status.PolicyError, status.ExitCode, status.Stderr)
	}
	if strings.TrimSpace(status.Stdout) != "" {
		t.Fatalf("old binding status = %q, want clean snapshot", status.Stdout)
	}
}

func TestGitShowRejectsOptionalRevision(t *testing.T) {
	ctx := context.Background()
	source := testGitRepo(t, map[string]string{"secret.txt": "secret\n"})
	manager := NewManager(Config{
		Root:  filepath.Join(t.TempDir(), "state"),
		Repos: []Repo{{ID: "project", Remote: source, DefaultRef: "main"}},
	})
	binding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}

	result := manager.Exec(ctx, ExecRequest{Command: "git show --name-only HEAD:secret.txt", Binding: binding})
	if result.PolicyError == "" {
		t.Fatalf("PolicyError is empty: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
	if strings.Contains(result.Stdout, "secret") {
		t.Fatalf("Stdout leaked blob contents: %q", result.Stdout)
	}
}

func TestSafeIDIsCollisionFreeForSimilarRepoIDs(t *testing.T) {
	if safeID("foo/bar") == safeID("foo_bar") {
		t.Fatal("safeID collided for distinct repo IDs")
	}
}

func TestExecRejectsUnsafeShellAndPathFeatures(t *testing.T) {
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := testGitRepo(t, map[string]string{"README.md": "hello\n"})
	if err := os.Symlink(outside, filepath.Join(source, "secret-link")); err != nil {
		t.Fatal(err)
	}
	git(t, source, "add", "secret-link")
	git(t, source, "commit", "-m", "add symlink")
	manager := NewManager(Config{
		Root: filepath.Join(t.TempDir(), "state"),
		Repos: []Repo{{
			ID:         "project",
			Remote:     source,
			DefaultRef: "main",
		}},
	})
	binding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}

	denied := []string{
		"rg hello . | head",
		"cat README.md > /tmp/out",
		"find . -type f -exec cat {} ;",
		"python -c 'print(1)'",
		"git clean -fdx",
		"rm -rf .",
		"cat /etc/passwd",
		"cat ../README.md",
		"cat $HOME",
		"cat `pwd`",
		"FOO=bar cat README.md",
		"cat secret-link",
		"grep hello",
		"rg --files --hidden",
		"tail -f README.md",
		"wc README.md",
	}
	for _, command := range denied {
		t.Run(command, func(t *testing.T) {
			result := manager.Exec(ctx, ExecRequest{Command: command, Binding: binding})
			if result.PolicyError == "" {
				t.Fatalf("PolicyError is empty: exit=%d stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
			}
		})
	}

	result := manager.Exec(ctx, ExecRequest{Command: "grep -R -n secret .", Binding: binding})
	if result.PolicyError != "" {
		t.Fatalf("PolicyError = %q", result.PolicyError)
	}
	if strings.Contains(result.Stdout, "secret") || strings.Contains(result.Stderr, "secret") {
		t.Fatalf("recursive grep followed symlink: stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
}

func TestExecCapsOutput(t *testing.T) {
	ctx := context.Background()
	source := testGitRepo(t, map[string]string{"README.md": strings.Repeat("x", 200)})
	manager := NewManager(Config{
		Root:           filepath.Join(t.TempDir(), "state"),
		MaxOutputBytes: 20,
		Repos:          []Repo{{ID: "project", Remote: source, DefaultRef: "main"}},
	})
	binding, err := manager.Bind(ctx, "project", []string{"project"})
	if err != nil {
		t.Fatal(err)
	}
	result := manager.Exec(ctx, ExecRequest{Command: "cat README.md", Binding: binding})
	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if !strings.Contains(result.Stdout, "[reposhell: output truncated]") {
		t.Fatalf("Stdout missing truncation marker: %q", result.Stdout)
	}
}

func mustExec(t *testing.T, manager Manager, binding Binding, command string) string {
	t.Helper()
	result := manager.Exec(context.Background(), ExecRequest{Command: command, Binding: binding})
	if result.PolicyError != "" || result.ExitCode != 0 {
		t.Fatalf("%s failed: policy=%q exit=%d stderr=%s", command, result.PolicyError, result.ExitCode, result.Stderr)
	}
	return result.Stdout
}

func testGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "localpager@example.invalid")
	git(t, dir, "config", "user.name", "Localpager Tests")
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "initial")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("/usr/bin/git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			t.Fatalf("git %s failed with %d:\n%s", strings.Join(args, " "), exitErr.ExitCode(), out)
		}
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}
