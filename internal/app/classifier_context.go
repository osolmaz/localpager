package app

import (
	"os"

	"github.com/osolmaz/localpager/internal/config"
	"github.com/osolmaz/localpager/internal/localpager"
)

func ClassifierContextOptionsFromConfig(cfg config.Config, githubBaseURL string, githubTokenEnv string) localpager.ClassifierContextOptions {
	github := cfg.Classifier.Context.GitHub
	opts := localpager.DefaultClassifierContextOptions()
	opts.IncludeBody = boolValue(github.IncludeBody, opts.IncludeBody)
	opts.IncludeLabels = boolValue(github.IncludeLabels, opts.IncludeLabels)
	opts.IncludeComments = boolValue(github.IncludeComments, opts.IncludeComments)
	opts.IncludeChangedFiles = boolValue(github.IncludeChangedFiles, opts.IncludeChangedFiles)
	opts.IncludeDiff = boolValue(github.IncludeDiff, opts.IncludeDiff)
	if github.MaxBodyChars > 0 {
		opts.MaxBodyChars = github.MaxBodyChars
	}
	if github.MaxCommentsChars > 0 {
		opts.MaxCommentsChars = github.MaxCommentsChars
	}
	if github.MaxChangedFilesChars > 0 {
		opts.MaxChangedFilesChars = github.MaxChangedFilesChars
	}
	if github.MaxDiffChars > 0 {
		opts.MaxDiffChars = github.MaxDiffChars
	}
	opts.GitHubBaseURL = githubBaseURL
	if githubTokenEnv == "" {
		githubTokenEnv = "GITHUB_TOKEN"
	}
	opts.GitHubToken = os.Getenv(githubTokenEnv)
	return opts
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
