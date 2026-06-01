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

	"github.com/osolmaz/localpager/internal/localpager"
	"github.com/osolmaz/localpager/internal/sources"
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

type EnqueueStats = sources.EnqueueStats

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func Enqueue(ctx context.Context, ingestor localpager.Ingestor, opts EnqueueOptions) (EnqueueStats, error) {
	if opts.Repo == "" {
		opts.Repo = localpager.DefaultRepo
	}
	if opts.Type == "" {
		opts.Type = "both"
	}
	if opts.ProcessorName == "" {
		opts.ProcessorName = localpager.DefaultProcessorName
	}
	if opts.ProcessorVersion == "" {
		opts.ProcessorVersion = localpager.DefaultProcessorVer
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
		result, err := ingestor.Ingest(ctx, item, localpager.IngestOptions{
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

func (client Client) List(ctx context.Context, repo, itemType string, limit int) ([]localpager.IngestItem, error) {
	if !sources.WantsPullRequests(itemType) && !sources.WantsIssues(itemType) {
		return nil, fmt.Errorf("unknown type %q", itemType)
	}
	var items []localpager.IngestItem
	if sources.WantsPullRequests(itemType) {
		pullRequests, err := client.listPullRequests(ctx, repo, limitForKind(limit, len(items)))
		if err != nil {
			return nil, err
		}
		items = append(items, pullRequests...)
	}
	if limit > 0 && len(items) >= limit {
		return items[:limit], nil
	}
	if sources.WantsIssues(itemType) {
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

func (client Client) listPullRequests(ctx context.Context, repo string, limit int) ([]localpager.IngestItem, error) {
	var pulls []pullRequest
	if err := client.listPages(ctx, repo, "pulls", limit, &pulls); err != nil {
		return nil, err
	}
	items := make([]localpager.IngestItem, 0, len(pulls))
	for _, pr := range pulls {
		items = append(items, mapPullRequest(repo, pr))
	}
	return items, nil
}

func (client Client) listIssues(ctx context.Context, repo string, limit int) ([]localpager.IngestItem, error) {
	var issues []issue
	if err := client.listPages(ctx, repo, "issues", limit, &issues); err != nil {
		return nil, err
	}
	items := make([]localpager.IngestItem, 0, len(issues))
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
	defer func() { _ = resp.Body.Close() }()
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
	Labels    []label   `json:"labels"`
}

type issue struct {
	Number      int             `json:"number"`
	State       string          `json:"state"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	HTMLURL     string          `json:"html_url"`
	UpdatedAt   time.Time       `json:"updated_at"`
	User        user            `json:"user"`
	Labels      []label         `json:"labels"`
	PullRequest json.RawMessage `json:"pull_request"`
}

type user struct {
	Login string `json:"login"`
}

type label struct {
	Name string `json:"name"`
}

func mapPullRequest(repo string, pr pullRequest) localpager.IngestItem {
	return mapGitHubItem(repo, "github_pr", pr.Number, pr.HTMLURL, pr.Title, pr.Body, labelsJSON(pr.Labels), pr.State, pr.User.Login, pr.UpdatedAt)
}

func mapIssue(repo string, issue issue) localpager.IngestItem {
	return mapGitHubItem(repo, "github_issue", issue.Number, issue.HTMLURL, issue.Title, issue.Body, labelsJSON(issue.Labels), issue.State, issue.User.Login, issue.UpdatedAt)
}

func mapGitHubItem(repo string, itemType string, number int, url string, title string, body string, labels string, state string, author string, updatedAt time.Time) localpager.IngestItem {
	item := localpager.IngestItem{
		Source:     "github",
		Type:       itemType,
		Ref:        fmt.Sprintf("%s#%d", repo, number),
		URL:        url,
		Title:      title,
		Body:       body,
		LabelsJSON: labels,
		State:      state,
		Author:     author,
		UpdatedAt:  updatedAt.UTC(),
		Metadata:   map[string]any{"repo": repo, "number": number},
	}
	item.ContentHash = contentHash(item)
	return item
}

func contentHash(item localpager.IngestItem) string {
	payload := []string{item.Type, item.Ref, item.Title, item.Body, item.LabelsJSON, item.State, item.Author, item.UpdatedAt.Format(time.RFC3339Nano)}
	sum := sha256.Sum256([]byte(strings.Join(payload, "\x00")))
	return hex.EncodeToString(sum[:])
}

func labelsJSON(labels []label) string {
	names := make([]string, 0, len(labels))
	for _, label := range labels {
		if strings.TrimSpace(label.Name) != "" {
			names = append(names, label.Name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	encoded, err := json.Marshal(names)
	if err != nil {
		return ""
	}
	return string(encoded)
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
