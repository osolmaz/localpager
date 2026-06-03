package reposhell

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeUnixRestrictsSocketPermissions(t *testing.T) {
	socketDir := filepath.Join(t.TempDir(), "socket")
	if err := os.MkdirAll(socketDir, 0o755); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(socketDir, "reposhell.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errs := make(chan error, 1)
	go func() {
		errs <- NewServer(NewManager(Config{})).ServeUnix(ctx, socketPath)
	}()
	waitForSocket(t, socketPath)

	dirInfo, err := os.Stat(socketDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("socket dir permissions = %o, want 700", got)
	}
	socketInfo, err := os.Stat(socketPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket permissions = %o, want 600", got)
	}

	cancel()
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestServerPrunesExpiredBindings(t *testing.T) {
	server := NewServer(NewManager(Config{}))
	server.bindingTTL = time.Minute
	server.mu.Lock()
	server.bindings["old"] = Binding{CWD: "/repo/old"}
	server.bindingCreated["old"] = time.Now().Add(-2 * time.Minute)
	server.bindings["fresh"] = Binding{CWD: "/repo/fresh"}
	server.bindingCreated["fresh"] = time.Now()
	server.pruneExpiredBindingsLocked(time.Now())
	server.mu.Unlock()

	if _, ok := server.bindings["old"]; ok {
		t.Fatal("old binding was not pruned")
	}
	if _, ok := server.bindings["fresh"]; !ok {
		t.Fatal("fresh binding was pruned")
	}
}

func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("socket was not created: %s", path)
}
