package app

import (
	"flag"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/localpager"
	"github.com/osolmaz/localpager/internal/sources/gitcrawl"
	githubsource "github.com/osolmaz/localpager/internal/sources/github"
)

type SourceCLIFields struct {
	DBPath           *string
	GitcrawlDBPath   *string
	GitHubBaseURL    *string
	GitHubTokenEnv   *string
	Repo             *string
	Type             *string
	ProcessorName    *string
	ProcessorVersion *string
	RecentWindow     *string
	CutoverAt        *string
}

func RegisterSourceCLIFlags(fs *flag.FlagSet, fields SourceCLIFields, typeUsage string) {
	fs.StringVar(fields.DBPath, "db", localpager.DefaultDBPath, "localpager SQLite database path")
	fs.StringVar(fields.GitcrawlDBPath, "gitcrawl-db", gitcrawl.DefaultDBPath, "gitcrawl SQLite database path")
	fs.StringVar(fields.GitHubBaseURL, "github-base-url", githubsource.DefaultBaseURL, "GitHub API base URL")
	fs.StringVar(fields.GitHubTokenEnv, "github-token-env", "GITHUB_TOKEN", "environment variable containing GitHub token")
	fs.StringVar(fields.Repo, "repo", localpager.DefaultRepo, "GitHub repo full name")
	fs.StringVar(fields.Type, "type", "both", typeUsage)
	fs.StringVar(fields.ProcessorName, "processor-name", localpager.DefaultProcessorName, "processor name")
	fs.StringVar(fields.ProcessorVersion, "processor-version", localpager.DefaultProcessorVer, "processor version")
	fs.StringVar(fields.RecentWindow, "recent-window", "48h", "duration considered recent for priority")
	fs.StringVar(fields.CutoverAt, "cutover-at", "", "RFC3339 timestamp; items updated before this are recorded as skipped")
}

func ApplySourceConfig(fields SourceCLIFields, cfg config.Config, setFlags map[string]bool, typeAliases ...string) {
	applyStringConfig(fields.DBPath, cfg.DBPath, setFlags, "db")
	applyStringConfig(fields.GitcrawlDBPath, cfg.GitcrawlDBPath, setFlags, "gitcrawl-db")
	ApplyGitHubConfig(fields.GitHubBaseURL, fields.GitHubTokenEnv, cfg, setFlags)
	applyStringConfig(fields.Repo, cfg.Repo, setFlags, "repo")
	applyStringConfigWithAliases(fields.Type, cfg.SourceType, setFlags, append([]string{"type"}, typeAliases...)...)
	applyStringConfig(fields.ProcessorName, cfg.ProcessorName, setFlags, "processor-name")
	applyStringConfig(fields.ProcessorVersion, cfg.ProcessorVersion, setFlags, "processor-version")
	applyStringConfig(fields.RecentWindow, cfg.RecentWindow, setFlags, "recent-window")
	applyStringConfig(fields.CutoverAt, cfg.CutoverAt, setFlags, "cutover-at")
}

func ApplyGitHubConfig(baseURL *string, tokenEnv *string, cfg config.Config, setFlags map[string]bool) {
	applyStringConfig(baseURL, cfg.GitHubBaseURL, setFlags, "github-base-url")
	applyStringConfig(tokenEnv, cfg.GitHubTokenEnv, setFlags, "github-token-env")
}

func applyStringConfig(target *string, value string, setFlags map[string]bool, flagName string) {
	if value != "" && !config.FlagSet(setFlags, flagName) {
		*target = value
	}
}

func applyStringConfigWithAliases(target *string, value string, setFlags map[string]bool, flagNames ...string) {
	if value == "" || anyFlagSet(setFlags, flagNames) {
		return
	}
	*target = value
}

func anyFlagSet(setFlags map[string]bool, flagNames []string) bool {
	for _, name := range flagNames {
		if config.FlagSet(setFlags, name) {
			return true
		}
	}
	return false
}
