package sources

import "testing"

func TestEnqueueStatsAddAndString(t *testing.T) {
	stats := EnqueueStats{ItemsSeen: 1, JobsInserted: 2}
	stats.Add(EnqueueStats{ItemsSeen: 3, ItemsUpserted: 4, JobsSkipped: 5, JobsExisting: 6})
	if stats.ItemsSeen != 4 || stats.ItemsUpserted != 4 || stats.JobsInserted != 2 || stats.JobsSkipped != 5 || stats.JobsExisting != 6 {
		t.Fatalf("stats = %+v", stats)
	}
	want := "items_seen=4 items_upserted=4 jobs_inserted=2 jobs_skipped=5 jobs_existing=6"
	if stats.String() != want {
		t.Fatalf("String() = %q, want %q", stats.String(), want)
	}
}

func TestWantsSourceKinds(t *testing.T) {
	t.Parallel()
	if !WantsPullRequests("prs") || !WantsPullRequests("both") || WantsPullRequests("issues") {
		t.Fatal("pull request type matching did not match expected aliases")
	}
	if !WantsIssues("issues") || !WantsIssues("all") || WantsIssues("pull_requests") {
		t.Fatal("issue type matching did not match expected aliases")
	}
}
