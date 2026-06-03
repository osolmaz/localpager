package reposhellcli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStandaloneExecReadsRepoConfig(t *testing.T) {
	repoRoot, head := currentRepo(t)
	configPath := writeConfig(t, repoRoot, head)
	var stdout, stderr bytes.Buffer

	code := Run([]string{
		"exec",
		"--config", configPath,
		"--command", "grep -n module go.mod",
	}, strings.NewReader(""), &stdout, &stderr, StandaloneOptions())

	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	if stdout.String() != "1:module github.com/osolmaz/localpager\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestStandaloneShellLoopsAndEnforcesPolicy(t *testing.T) {
	repoRoot, head := currentRepo(t)
	configPath := writeConfig(t, repoRoot, head)
	var stdout, stderr bytes.Buffer

	code := Run([]string{
		"shell",
		"--config", configPath,
	}, strings.NewReader("pwd\ngrep -n module go.mod\ncat /etc/passwd\nexit\n"), &stdout, &stderr, StandaloneOptions())

	if code != 0 {
		t.Fatalf("exit code = %d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{
		"reposhell bound cwd=/repo/project repos=project",
		"reposhell /repo/project> /repo/project\n",
		"1:module github.com/osolmaz/localpager\n",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, missing %q", stdout.String(), want)
		}
	}
	if !strings.Contains(stderr.String(), "absolute paths outside /repo are not allowed") {
		t.Fatalf("stderr = %q, missing policy denial", stderr.String())
	}
	if !strings.Contains(stderr.String(), "exit_code=2") {
		t.Fatalf("stderr = %q, missing exit code", stderr.String())
	}
}

func TestLoadConfigAcceptsLocalpagerNestedReposhell(t *testing.T) {
	repoRoot, head := currentRepo(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "localpager.json")
	body := `{
  "classifier": {
    "reposhell_default_repo": "project",
    "reposhell_visible_repos": ["project"]
  },
  "reposhell": {
    "root": "` + filepath.ToSlash(filepath.Join(dir, "state")) + `",
    "socket": "` + filepath.ToSlash(filepath.Join(dir, "reposhell.sock")) + `",
    "snapshot_retain": 3,
    "repos": [{"id": "project", "remote": "` + filepath.ToSlash(repoRoot) + `", "default_ref": "` + head + `"}]
  }
}`
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath, LocalpagerOptions())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultRepo != "project" {
		t.Fatalf("DefaultRepo = %q", cfg.DefaultRepo)
	}
	if len(cfg.VisibleRepos) != 1 || cfg.VisibleRepos[0] != "project" {
		t.Fatalf("VisibleRepos = %#v", cfg.VisibleRepos)
	}
	if cfg.Socket != filepath.Join(dir, "reposhell.sock") {
		t.Fatalf("Socket = %q", cfg.Socket)
	}
	if cfg.ManagerConfig.SnapshotRetain != 3 {
		t.Fatalf("SnapshotRetain = %d, want 3", cfg.ManagerConfig.SnapshotRetain)
	}
}

func writeConfig(t *testing.T, source, head string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "reposhell.json")
	body := `{
  "root": "` + filepath.ToSlash(filepath.Join(dir, "state")) + `",
  "socket": "` + filepath.ToSlash(filepath.Join(dir, "reposhell.sock")) + `",
  "default_repo": "project",
  "visible_repos": ["project"],
  "repos": [{"id": "project", "remote": "` + filepath.ToSlash(source) + `", "default_ref": "` + head + `"}]
}`
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return configPath
}

func currentRepo(t *testing.T) (string, string) {
	t.Helper()
	rootRaw, err := exec.Command("/usr/bin/git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	headRaw, err := exec.Command("/usr/bin/git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("resolve repo head: %v", err)
	}
	return strings.TrimSpace(string(rootRaw)), strings.TrimSpace(string(headRaw))
}
