package sources

func WantsPullRequests(itemType string) bool {
	switch itemType {
	case "pr", "prs", "pull_request", "pull_requests", "github_pr", "both", "all", "":
		return true
	default:
		return false
	}
}

func WantsIssues(itemType string) bool {
	switch itemType {
	case "issue", "issues", "github_issue", "both", "all", "":
		return true
	default:
		return false
	}
}
