package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/osolmaz/localpager/internal/notifier"
)

func TestEnqueueMapsGitHubAPIItems(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/repo/pulls":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{
				"number": 7,
				"state": "open",
				"title": "PR title",
				"body": "PR body",
				"html_url": "https://github.com/example/repo/pull/7",
				"updated_at": "2026-06-01T10:00:00Z",
				"user": {"login": "alice"}
			}]`))
		case "/repos/example/repo/issues":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{
				"number": 8,
				"state": "open",
				"title": "Issue title",
				"body": "Issue body",
				"html_url": "https://github.com/example/repo/issues/8",
				"updated_at": "2026-06-01T11:00:00Z",
				"user": {"login": "bob"}
			}, {
				"number": 7,
				"state": "open",
				"title": "PR duplicate from issues endpoint",
				"pull_request": {}
			}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	pool, err := notifier.NewPool(ctx, filepath.Join(dir, "notifier.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	stats, err := Enqueue(ctx, notifier.NewIngestor(pool), EnqueueOptions{
		Repo:    "example/repo",
		Type:    "both",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.ItemsSeen != 2 || stats.JobsInserted != 2 {
		t.Fatalf("stats = %+v, want 2 seen and 2 jobs", stats)
	}

	var items []notifier.Item
	if err := pool.GORM().Order("source_kind").Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Source == nil || *items[0].Source != "github" {
		t.Fatalf("item source = %v, want github", items[0].Source)
	}
}
