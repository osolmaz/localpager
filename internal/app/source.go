package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
	"github.com/osolmaz/localpager/internal/sources"
	"github.com/osolmaz/localpager/internal/sources/gitcrawl"
	githubsource "github.com/osolmaz/localpager/internal/sources/github"
)

type SourceOptions struct {
	Source           string
	Repo             string
	Type             string
	Limit            int
	InitialHydration bool
	ProcessorName    string
	ProcessorVersion string
	RecentWindow     time.Duration
	CutoverAt        *time.Time
	GitcrawlDBPath   string
	GitHubBaseURL    string
	GitHubTokenEnv   string
}

func EnqueueSource(ctx context.Context, ingestor notifier.Ingestor, opts SourceOptions) (sources.EnqueueStats, error) {
	switch opts.Source {
	case "gitcrawl":
		return enqueueGitcrawl(ctx, ingestor, opts)
	case "github":
		return enqueueGitHub(ctx, ingestor, opts)
	default:
		return sources.EnqueueStats{}, fmt.Errorf("unsupported source %q", opts.Source)
	}
}

func enqueueGitcrawl(ctx context.Context, ingestor notifier.Ingestor, opts SourceOptions) (sources.EnqueueStats, error) {
	db, err := gitcrawl.OpenDB(ctx, opts.GitcrawlDBPath)
	if err != nil {
		return sources.EnqueueStats{}, err
	}
	defer closeSQLDB(db)
	return gitcrawl.Enqueue(ctx, ingestor, db, gitcrawl.EnqueueOptions{
		Repo:             opts.Repo,
		Type:             opts.Type,
		Limit:            opts.Limit,
		InitialHydration: opts.InitialHydration,
		ProcessorName:    opts.ProcessorName,
		ProcessorVersion: opts.ProcessorVersion,
		RecentWindow:     opts.RecentWindow,
		CutoverAt:        opts.CutoverAt,
	})
}

func enqueueGitHub(ctx context.Context, ingestor notifier.Ingestor, opts SourceOptions) (sources.EnqueueStats, error) {
	return githubsource.Enqueue(ctx, ingestor, githubsource.EnqueueOptions{
		Repo:             opts.Repo,
		Type:             opts.Type,
		Limit:            opts.Limit,
		InitialHydration: opts.InitialHydration,
		ProcessorName:    opts.ProcessorName,
		ProcessorVersion: opts.ProcessorVersion,
		RecentWindow:     opts.RecentWindow,
		CutoverAt:        opts.CutoverAt,
		BaseURL:          opts.GitHubBaseURL,
		Token:            os.Getenv(opts.GitHubTokenEnv),
	})
}

func closeSQLDB(db *sql.DB) {
	_ = db.Close()
}
