package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
)

const DefaultBaseURL = "https://api.github.com"

type EnqueueOptions struct {
	Repo             string
	Type             string
	Limit            int
	InitialHydration bool
	ProcessorName    string
	ProcessorVersion string
	RecentWindow     time.Duration
	CutoverAt        *time.Time
	BaseURL          string
	Token            string
}

type EnqueueStats struct {
	ItemsSeen     int
	ItemsUpserted int
	JobsInserted  int
	JobsSkipped   int
	JobsExisting  int
}

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func Enqueue(ctx context.Context, ingestor notifier.Ingestor, opts EnqueueOptions) (EnqueueStats, error) {
	if opts.Repo == "" {
		opts.Repo = notifier.DefaultRepo
	}
	if opts.Type == "" {
		opts.Type = "both"
	}
	if opts.ProcessorName == "" {
		opts.ProcessorName = notifier.DefaultProcessorName
	}
	if opts.ProcessorVersion == "" {
		opts.ProcessorVersion = notifier.DefaultProcessorVer
	}
	if opts.RecentWindow == 0 {
		opts.RecentWindow = 48 * time.Hour
	}

	client := Client{BaseURL: opts.BaseURL, Token: opts.Token}
	items, err := client.List(ctx, opts.Repo, opts.Type, opts.Limit)
	if err != nil {
		return EnqueueStats{}, err
	}
	stats := EnqueueStats{ItemsSeen: len(items)}
	for _, item := range items {
		result, err := ingestor.Ingest(ctx, item, notifier.IngestOptions{
			JobType:          "classify_" + item.Type,
			ProcessorName:    opts.ProcessorName,
			ProcessorVersion: opts.ProcessorVersion,
			Priority:         priorityFor(item.Type, item.UpdatedAt, opts.RecentWindow),
			InitialHydration: opts.InitialHydration,
			CutoverAt:        opts.CutoverAt,
		})
		if err != nil {
			return stats, err
		}
		stats.ItemsUpserted++
		if result.JobInserted {
			stats.JobsInserted++
		}
		if result.JobSkipped {
			stats.JobsSkipped++
		}
		if result.JobExisting {
			stats.JobsExisting++
		}
	}
	return stats, nil
}

func (client Client) List(ctx context.Context, repo, itemType string, limit int) ([]notifier.IngestItem, error) {
	if !wantsPullRequests(itemType) && !wantsIssues(itemType) {
		return nil, fmt.Errorf("unknown type %q", itemType)
	}
	var items []notifier.IngestItem
	if wantsPullRequests(itemType) {
		pullRequests, err := client.listPullRequests(ctx, repo, limitForKind(limit, len(items)))
		if err != nil {
			return nil, err
		}
		items = append(items, pullRequests...)
	}
	if limit > 0 && len(items) >= limit {
		return items[:limit], nil
	}
	if wantsIssues(itemType) {
		issues, err := client.listIssues(ctx, repo, limitForKind(limit, len(items)))
		if err != nil {
			return nil, err
		}
		items = append(items, issues...)
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (client Client) listPullRequests(ctx context.Context, repo string, limit int) ([]notifier.IngestItem, error) {
	var pulls []pullRequest
	if err := client.listPages(ctx, repo, "pulls", limit, &pulls); err != nil {
		return nil, err
	}
	items := make([]notifier.IngestItem, 0, len(pulls))
	for _, pr := range pulls {
		items = append(items, mapPullRequest(repo, pr))
	}
	return items, nil
}

func (client Client) listIssues(ctx context.Context, repo string, limit int) ([]notifier.IngestItem, error) {
	var issues []issue
	if err := client.listPages(ctx, repo, "issues", limit, &issues); err != nil {
		return nil, err
	}
	items := make([]notifier.IngestItem, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequest != nil {
			continue
		}
		items = append(items, mapIssue(repo, issue))
	}
	return items, nil
}

func (client Client) listPages(ctx context.Context, repo, endpoint string, limit int, out any) error {
	base := strings.TrimRight(client.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	perPage := 100
	if limit > 0 && limit < perPage {
		perPage = limit
	}
	values := url.Values{}
	values.Set("state", "open")
	values.Set("sort", "updated")
	values.Set("direction", "desc")
	values.Set("per_page", strconv.Itoa(perPage))
	values.Set("page", "1")
	requestURL := fmt.Sprintf("%s/repos/%s/%s?%s", base, repo, endpoint, values.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "localpager")
	if client.Token != "" {
		req.Header.Set("Authorization", "Bearer "+client.Token)
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github %s returned status %d", endpoint, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type pullRequest struct {
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	UpdatedAt time.Time `json:"updated_at"`
	User      user      `json:"user"`
}

type issue struct {
	Number      int             `json:"number"`
	State       string          `json:"state"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	HTMLURL     string          `json:"html_url"`
	UpdatedAt   time.Time       `json:"updated_at"`
	User        user            `json:"user"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type user struct {
	Login string `json:"login"`
}

func mapPullRequest(repo string, pr pullRequest) notifier.IngestItem {
	item := notifier.IngestItem{
		Source:    "github",
		Type:      "github_pr",
		Ref:       fmt.Sprintf("%s#%d", repo, pr.Number),
		URL:       pr.HTMLURL,
		Title:     pr.Title,
		Body:      pr.Body,
		State:     pr.State,
		Author:    pr.User.Login,
		UpdatedAt: pr.UpdatedAt.UTC(),
		Metadata:  map[string]any{"repo": repo, "number": pr.Number},
	}
	item.ContentHash = contentHash(item)
	return item
}

func mapIssue(repo string, issue issue) notifier.IngestItem {
	item := notifier.IngestItem{
		Source:    "github",
		Type:      "github_issue",
		Ref:       fmt.Sprintf("%s#%d", repo, issue.Number),
		URL:       issue.HTMLURL,
		Title:     issue.Title,
		Body:      issue.Body,
		State:     issue.State,
		Author:    issue.User.Login,
		UpdatedAt: issue.UpdatedAt.UTC(),
		Metadata:  map[string]any{"repo": repo, "number": issue.Number},
	}
	item.ContentHash = contentHash(item)
	return item
}

func contentHash(item notifier.IngestItem) string {
	payload := []string{item.Type, item.Ref, item.Title, item.Body, item.State, item.Author, item.UpdatedAt.Format(time.RFC3339Nano)}
	sum := sha256.Sum256([]byte(strings.Join(payload, "\x00")))
	return hex.EncodeToString(sum[:])
}

func wantsPullRequests(itemType string) bool {
	switch itemType {
	case "pr", "prs", "pull_request", "pull_requests", "github_pr", "both", "all", "":
		return true
	default:
		return false
	}
}

func wantsIssues(itemType string) bool {
	switch itemType {
	case "issue", "issues", "github_issue", "both", "all", "":
		return true
	default:
		return false
	}
}

func limitForKind(limit, seen int) int {
	if limit <= 0 {
		return 0
	}
	remaining := limit - seen
	if remaining < 0 {
		return 0
	}
	return remaining
}

func priorityFor(itemType string, updatedAt time.Time, recentWindow time.Duration) int {
	base := 20
	old := 120
	if itemType == "github_pr" {
		base = 10
		old = 100
	}
	if time.Since(updatedAt) <= recentWindow {
		return base
	}
	return old
}
