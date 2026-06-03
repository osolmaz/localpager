package main

import (
	"testing"

	"github.com/osolmaz/localpager/internal/config"
)

func TestLocalpagerReposhellRuntimeConfigAdaptsLocalpagerConfig(t *testing.T) {
	runtime, err := localpagerReposhellRuntimeConfig(config.Config{
		Classifier: config.Classifier{
			ReposhellDefaultRepo:  "project",
			ReposhellVisibleRepos: []string{"project", "docs"},
		},
		Reposhell: config.Reposhell{
			Socket:          "/tmp/localpager-reposhell.sock",
			Root:            "/tmp/localpager-reposhell",
			RefreshInterval: "24h",
			CommandTimeout:  "2s",
			MaxOutputBytes:  1234,
			SnapshotRetain:  7,
			Repos: []config.ReposhellRepo{{
				ID:              "project",
				Remote:          "https://example.invalid/project.git",
				DefaultRef:      "origin/main",
				RefreshInterval: "1h",
			}},
		},
	}, localpagerReposhellOptions())
	if err != nil {
		t.Fatal(err)
	}

	if runtime.Socket != "/tmp/localpager-reposhell.sock" {
		t.Fatalf("Socket = %q", runtime.Socket)
	}
	if runtime.DefaultRepo != "project" {
		t.Fatalf("DefaultRepo = %q", runtime.DefaultRepo)
	}
	if len(runtime.VisibleRepos) != 2 || runtime.VisibleRepos[1] != "docs" {
		t.Fatalf("VisibleRepos = %#v", runtime.VisibleRepos)
	}
	if runtime.ManagerConfig.Root != "/tmp/localpager-reposhell" {
		t.Fatalf("Root = %q", runtime.ManagerConfig.Root)
	}
	if runtime.ManagerConfig.MaxOutputBytes != 1234 {
		t.Fatalf("MaxOutputBytes = %d", runtime.ManagerConfig.MaxOutputBytes)
	}
	if runtime.ManagerConfig.SnapshotRetain != 7 {
		t.Fatalf("SnapshotRetain = %d", runtime.ManagerConfig.SnapshotRetain)
	}
	if len(runtime.ManagerConfig.Repos) != 1 || runtime.ManagerConfig.Repos[0].ID != "project" {
		t.Fatalf("Repos = %#v", runtime.ManagerConfig.Repos)
	}
}
