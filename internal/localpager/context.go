package localpager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const defaultGitHubBaseURL = "https://api.github.com"

var diffKeywords = []string{
	"localagent",
	"local model",
	"local-model",
	"lm studio",
	"lmstudio",
	"vllm",
	"ollama",
	"llama.cpp",
	"gemma",
	"gitcrawl",
	"classifier",
	"topics_of_interest",
	"final_json",
	"final-schema",
	"mcp",
	"acp",
	"acpx",
	"codex",
	"huggingface",
	"hf",
	"hub workflow",
	"model serving",
	"open weight",
	"self-hosted",
	"post training",
}

func DefaultClassifierContextOptions() ClassifierContextOptions {
	return ClassifierContextOptions{
		IncludeBody:          true,
		IncludeLabels:        true,
		IncludeComments:      true,
		IncludeChangedFiles:  true,
		IncludeDiff:          true,
		MaxBodyChars:         2500,
		MaxCommentsChars:     1500,
		MaxChangedFilesChars: 2000,
		MaxDiffChars:         5000,
		GitHubBaseURL:        defaultGitHubBaseURL,
	}
}

func RenderGitHubTargetContext(ctx context.Context, target string, opts ClassifierContextOptions) (string, error) {
	repo, number, err := parseGitHubTarget(target)
	if err != nil {
		return "", err
	}
	var issue struct {
		HTMLURL string `json:"html_url"`
		Title   string `json:"title"`
		State   string `json:"state"`
		Body    string `json:"body"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		PullRequest *struct{} `json:"pull_request"`
	}
	if err := githubJSON(ctx, opts, fmt.Sprintf("/repos/%s/issues/%d", repo, number), &issue); err != nil {
		return "", fmt.Errorf("github item unavailable: %w", err)
	}
	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		labels = append(labels, label.Name)
	}
	labelsJSON, err := json.Marshal(compactStrings(labels))
	if err != nil {
		return "", err
	}
	itemType := "github_issue"
	urlSuffix := "issues"
	if issue.PullRequest != nil {
		itemType = "github_pr"
		urlSuffix = "pull"
	}
	ref := fmt.Sprintf("%s#%d", repo, number)
	sourceURL := issue.HTMLURL
	if sourceURL == "" {
		sourceURL = fmt.Sprintf("https://github.com/%s/%s/%d", repo, urlSuffix, number)
	}
	metadataJSON, err := json.Marshal(map[string]any{
		"repo":   repo,
		"number": number,
	})
	if err != nil {
		return "", err
	}
	return renderClassifierContext(ctx, Item{
		SourceKind:   "github",
		SourceRef:    ref,
		SourceURL:    stringPtrOrNil(sourceURL),
		Type:         stringPtr(itemType),
		Ref:          stringPtr(ref),
		Title:        stringPtrOrNil(issue.Title),
		Body:         stringPtrOrNil(issue.Body),
		LabelsJSON:   stringPtrOrNil(string(labelsJSON)),
		State:        stringPtrOrNil(issue.State),
		Author:       stringPtrOrNil(issue.User.Login),
		MetadataJSON: stringPtr(string(metadataJSON)),
	}, opts), nil
}

func parseGitHubTarget(target string) (string, int, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", 0, fmt.Errorf("github target is empty")
	}
	if repo, number := parseRepoNumber(target); repo != "" && number != 0 {
		return repo, number, nil
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", 0, fmt.Errorf("unsupported github target %q; use owner/repo#number or a GitHub issue/PR URL", target)
	}
	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return "", 0, fmt.Errorf("unsupported github host %q", parsed.Host)
	}
	parts := compactStrings(strings.Split(strings.Trim(parsed.Path, "/"), "/"))
	if len(parts) < 4 || (parts[2] != "issues" && parts[2] != "pull") {
		return "", 0, fmt.Errorf("unsupported github target %q; use owner/repo#number or a GitHub issue/PR URL", target)
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number == 0 {
		return "", 0, fmt.Errorf("unsupported github target %q; issue/PR number is invalid", target)
	}
	return parts[0] + "/" + parts[1], number, nil
}

func renderClassifierContext(ctx context.Context, item Item, opts ClassifierContextOptions) string {
	opts = normalizeClassifierContextOptions(opts)
	meta := githubItemMetadata(item)
	var caveats []string
	comments, commentsTruncated, err := githubComments(ctx, meta, opts)
	if err != nil {
		caveats = append(caveats, err.Error())
	}
	files, filesTruncated, err := githubChangedFiles(ctx, meta, opts)
	if err != nil {
		caveats = append(caveats, err.Error())
	}
	diff, diffTruncated, err := githubDiff(ctx, meta, opts)
	if err != nil {
		caveats = append(caveats, err.Error())
	}

	var b strings.Builder
	b.WriteString("GitHub item:\n")
	writeContextLine(&b, "Repository", meta.repo)
	writeContextLine(&b, "Type", meta.itemType)
	if meta.number != 0 {
		writeContextLine(&b, "Number", strconv.Itoa(meta.number))
	}
	writeContextLine(&b, "URL", deref(item.SourceURL))
	writeContextLine(&b, "Title", neutralizeControlTags(deref(item.Title)))
	writeContextLine(&b, "State", deref(item.State))
	writeContextLine(&b, "Author", deref(item.Author))
	if opts.IncludeLabels {
		writeContextLine(&b, "Labels", strings.Join(labelNames(deref(item.LabelsJSON)), ", "))
	}
	if files != "" {
		label := "Changed files"
		if filesTruncated {
			label += " (truncated)"
		}
		writeContextLine(&b, label, files)
	}
	if len(caveats) > 0 {
		writeContextLine(&b, "Context caveats", strings.Join(caveats, ", "))
	}
	if opts.IncludeBody {
		body := truncateText(neutralizeControlTags(deref(item.Body)), opts.MaxBodyChars, "body")
		b.WriteString("\nBody")
		if body.truncated {
			b.WriteString(" (truncated)")
		}
		b.WriteString(":\n```markdown\n")
		b.WriteString(body.text)
		b.WriteString("\n```\n")
	}
	if comments != "" {
		b.WriteString("\nComments/context")
		if commentsTruncated {
			b.WriteString(" (truncated)")
		}
		b.WriteString(":\n```markdown\n")
		b.WriteString(comments)
		b.WriteString("\n```\n")
	}
	if diff != "" {
		b.WriteString("\nDiff/context")
		if diffTruncated {
			b.WriteString(" (selected/truncated)")
		}
		b.WriteString(":\n```diff\n")
		b.WriteString(diff)
		b.WriteString("\n```\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeClassifierContextOptions(opts ClassifierContextOptions) ClassifierContextOptions {
	defaults := DefaultClassifierContextOptions()
	if opts.MaxBodyChars <= 0 {
		opts.MaxBodyChars = defaults.MaxBodyChars
	}
	if opts.MaxCommentsChars <= 0 {
		opts.MaxCommentsChars = defaults.MaxCommentsChars
	}
	if opts.MaxChangedFilesChars <= 0 {
		opts.MaxChangedFilesChars = defaults.MaxChangedFilesChars
	}
	if opts.MaxDiffChars <= 0 {
		opts.MaxDiffChars = defaults.MaxDiffChars
	}
	if strings.TrimSpace(opts.GitHubBaseURL) == "" {
		opts.GitHubBaseURL = defaults.GitHubBaseURL
	}
	return opts
}

func writeContextLine(b *strings.Builder, label string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	fmt.Fprintf(b, "- %s: %s\n", label, value)
}

type githubMetadata struct {
	repo     string
	number   int
	itemType string
}

func githubItemMetadata(item Item) githubMetadata {
	meta := githubMetadata{itemType: deref(item.Type)}
	if meta.itemType == "" {
		meta.itemType = item.SourceKind
	}
	if item.MetadataJSON != nil {
		var raw map[string]any
		if err := json.Unmarshal([]byte(*item.MetadataJSON), &raw); err == nil {
			if repo, ok := raw["repo"].(string); ok {
				meta.repo = repo
			}
			switch number := raw["number"].(type) {
			case float64:
				meta.number = int(number)
			case string:
				meta.number, _ = strconv.Atoi(number)
			}
		}
	}
	if meta.repo == "" || meta.number == 0 {
		repo, number := parseRepoNumber(deref(item.Ref))
		if repo == "" || number == 0 {
			repo, number = parseRepoNumber(item.SourceRef)
		}
		if meta.repo == "" {
			meta.repo = repo
		}
		if meta.number == 0 {
			meta.number = number
		}
	}
	return meta
}

func parseRepoNumber(ref string) (string, int) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", 0
	}
	ref = strings.TrimPrefix(ref, "gitcrawl:")
	repo, numberText, ok := strings.Cut(ref, "#")
	if !ok {
		return "", 0
	}
	number, err := strconv.Atoi(numberText)
	if err != nil {
		return "", 0
	}
	return repo, number
}

func labelNames(labelsJSON string) []string {
	labelsJSON = strings.TrimSpace(labelsJSON)
	if labelsJSON == "" {
		return nil
	}
	var names []string
	if err := json.Unmarshal([]byte(labelsJSON), &names); err == nil {
		return compactStrings(names)
	}
	var objects []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(labelsJSON), &objects); err == nil {
		for _, object := range objects {
			names = append(names, object.Name)
		}
	}
	return compactStrings(names)
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func githubComments(ctx context.Context, meta githubMetadata, opts ClassifierContextOptions) (string, bool, error) {
	if !opts.IncludeComments || meta.repo == "" || meta.number == 0 {
		return "", false, nil
	}
	var comments []struct {
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := githubJSON(ctx, opts, fmt.Sprintf("/repos/%s/issues/%d/comments", meta.repo, meta.number), &comments); err != nil {
		return "", false, fmt.Errorf("comments unavailable: %w", err)
	}
	parts := make([]string, 0, len(comments))
	for _, comment := range comments {
		author := comment.User.Login
		if author == "" {
			author = "unknown"
		}
		when := ""
		if !comment.CreatedAt.IsZero() {
			when = " at " + comment.CreatedAt.Format(time.RFC3339)
		}
		parts = append(parts, fmt.Sprintf("- %s%s:\n%s", author, when, comment.Body))
	}
	truncated := truncateText(neutralizeControlTags(strings.Join(parts, "\n\n")), opts.MaxCommentsChars, "comments/context")
	return truncated.text, truncated.truncated, nil
}

func githubChangedFiles(ctx context.Context, meta githubMetadata, opts ClassifierContextOptions) (string, bool, error) {
	if !opts.IncludeChangedFiles || meta.repo == "" || meta.number == 0 || meta.itemType != "github_pr" {
		return "", false, nil
	}
	var files []struct {
		Filename string `json:"filename"`
	}
	if err := githubJSON(ctx, opts, fmt.Sprintf("/repos/%s/pulls/%d/files", meta.repo, meta.number), &files); err != nil {
		return "", false, fmt.Errorf("changed files unavailable: %w", err)
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, file.Filename)
	}
	truncated := truncateText(neutralizeControlTags(strings.Join(compactStrings(names), ", ")), opts.MaxChangedFilesChars, "changed files")
	return truncated.text, truncated.truncated, nil
}

func githubDiff(ctx context.Context, meta githubMetadata, opts ClassifierContextOptions) (string, bool, error) {
	if !opts.IncludeDiff || meta.repo == "" || meta.number == 0 || meta.itemType != "github_pr" {
		return "", false, nil
	}
	diff, err := githubText(ctx, opts, fmt.Sprintf("/repos/%s/pulls/%d", meta.repo, meta.number), "application/vnd.github.v3.diff")
	if err != nil {
		return "", false, fmt.Errorf("diff unavailable: %w", err)
	}
	selected := selectDiff(neutralizeControlTags(diff), opts.MaxDiffChars)
	return selected.text, selected.truncated, nil
}

func githubJSON(ctx context.Context, opts ClassifierContextOptions, endpoint string, out any) error {
	body, err := githubRequest(ctx, opts, endpoint, "application/vnd.github+json")
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func githubText(ctx context.Context, opts ClassifierContextOptions, endpoint string, accept string) (string, error) {
	body, err := githubRequest(ctx, opts, endpoint, accept)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func githubRequest(ctx context.Context, opts ClassifierContextOptions, endpoint string, accept string) ([]byte, error) {
	base := strings.TrimRight(opts.GitHubBaseURL, "/")
	if base == "" {
		base = defaultGitHubBaseURL
	}
	requestURL := base + endpoint
	if strings.Contains(endpoint, "?") {
		requestURL += "&per_page=100"
	} else {
		requestURL += "?per_page=100"
	}
	parsed, err := url.Parse(requestURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "localpager")
	if opts.GitHubToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.GitHubToken)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github returned status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type truncatedText struct {
	text      string
	truncated bool
}

func truncateText(text string, maxChars int, label string) truncatedText {
	if maxChars <= 0 || len(text) <= maxChars {
		return truncatedText{text: text}
	}
	headSize := maxChars * 7 / 10
	tailSize := maxChars - headSize - 120
	if tailSize < 0 {
		tailSize = 0
	}
	return truncatedText{
		text:      fmt.Sprintf("%s\n\n[%s truncated: %d characters omitted]\n\n%s", text[:headSize], label, len(text)-headSize-tailSize, text[len(text)-tailSize:]),
		truncated: true,
	}
}

func selectDiff(diff string, maxChars int) truncatedText {
	if maxChars <= 0 || len(diff) <= maxChars {
		return truncatedText{text: diff}
	}
	chunks := diffChunks(diff)
	type scoredChunk struct {
		text  string
		index int
		score int
	}
	scored := make([]scoredChunk, 0, len(chunks))
	for index, chunk := range chunks {
		scored = append(scored, scoredChunk{text: chunk, index: index, score: diffScore(chunk)})
	}
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score || (scored[j].score == scored[i].score && scored[j].index < scored[i].index) {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	var selected []scoredChunk
	used := 0
	for _, chunk := range scored {
		if used >= maxChars {
			break
		}
		remaining := maxChars - used
		perChunkLimit := min(remaining, max(2400, maxChars/4))
		truncated := truncateText(chunk.text, perChunkLimit, "file diff")
		selected = append(selected, scoredChunk{text: truncated.text, index: chunk.index, score: chunk.score})
		used += len(truncated.text) + 2
	}
	for i := 0; i < len(selected); i++ {
		for j := i + 1; j < len(selected); j++ {
			if selected[j].index < selected[i].index {
				selected[i], selected[j] = selected[j], selected[i]
			}
		}
	}
	parts := make([]string, 0, len(selected))
	for _, chunk := range selected {
		parts = append(parts, chunk.text)
	}
	return truncatedText{
		text:      fmt.Sprintf("%s\n\n[diff truncated from %d characters to selected relevant excerpts]", strings.Join(parts, "\n\n"), len(diff)),
		truncated: true,
	}
}

func diffChunks(diff string) []string {
	parts := strings.Split(diff, "\ndiff --git ")
	if len(parts) <= 1 {
		return []string{diff}
	}
	chunks := make([]string, 0, len(parts))
	if strings.TrimSpace(parts[0]) != "" {
		chunks = append(chunks, parts[0])
	}
	for _, part := range parts[1:] {
		chunks = append(chunks, "diff --git "+part)
	}
	return chunks
}

func diffScore(chunk string) int {
	lower := strings.ToLower(chunk)
	score := 0
	for _, keyword := range diffKeywords {
		if strings.Contains(lower, keyword) {
			score += 10
		}
	}
	if strings.Contains(lower, "schema") || strings.Contains(lower, "template") {
		score += 5
	}
	if strings.Contains(lower, "diff --git") {
		score++
	}
	return score
}

func neutralizeControlTags(text string) string {
	re := regexp.MustCompile(`(?i)</?(?:think|final|analysis|assistant|system|user)\b[^>]*>`)
	return re.ReplaceAllStringFunc(text, func(tag string) string {
		tag = strings.ReplaceAll(tag, "<", "&lt;")
		return strings.ReplaceAll(tag, ">", "&gt;")
	})
}
