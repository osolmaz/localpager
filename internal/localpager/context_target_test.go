package localpager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderGitHubTargetContextFetchesIssueThenDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/example/repo/issues/9":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/example/repo/pull/9","title":"Use reposhell","state":"open","body":"Needs lm studio routing.","user":{"login":"alice"},"labels":[{"name":"local_models"}],"pull_request":{}}`))
		case "/repos/example/repo/issues/9/comments":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"body":"Read files with bash.","created_at":"2026-06-01T00:00:00Z","user":{"login":"bob"}}]`))
		case "/repos/example/repo/pulls/9/files":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"filename":"cmd/reposhell/main.go"}]`))
		case "/repos/example/repo/pulls/9":
			_, _ = w.Write([]byte("diff --git a/cmd/reposhell/main.go b/cmd/reposhell/main.go\n+reposhell bash\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	rendered, err := RenderGitHubTargetContext(context.Background(), "example/repo#9", ClassifierContextOptions{
		IncludeBody:          true,
		IncludeLabels:        true,
		IncludeComments:      true,
		IncludeChangedFiles:  true,
		IncludeDiff:          true,
		MaxBodyChars:         2500,
		MaxCommentsChars:     1500,
		MaxChangedFilesChars: 2000,
		MaxDiffChars:         5000,
		GitHubBaseURL:        server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Repository: example/repo",
		"Type: github_pr",
		"Title: Use reposhell",
		"Labels: local_models",
		"Changed files: cmd/reposhell/main.go",
		"Read files with bash.",
		"reposhell bash",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered context missing %q:\n%s", want, rendered)
		}
	}
}

func TestRenderGitHubTargetContextAcceptsGitHubURL(t *testing.T) {
	repo, number, err := parseGitHubTarget("https://github.com/example/repo/pull/42")
	if err != nil {
		t.Fatal(err)
	}
	if repo != "example/repo" || number != 42 {
		t.Fatalf("parseGitHubTarget = %s#%d, want example/repo#42", repo, number)
	}
}
