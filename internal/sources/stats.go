package sources

import "fmt"

type EnqueueStats struct {
	ItemsSeen     int
	ItemsUpserted int
	JobsInserted  int
	JobsSkipped   int
	JobsExisting  int
}

func (stats *EnqueueStats) Add(other EnqueueStats) {
	stats.ItemsSeen += other.ItemsSeen
	stats.ItemsUpserted += other.ItemsUpserted
	stats.JobsInserted += other.JobsInserted
	stats.JobsSkipped += other.JobsSkipped
	stats.JobsExisting += other.JobsExisting
}

func (stats EnqueueStats) String() string {
	return fmt.Sprintf(
		"items_seen=%d items_upserted=%d jobs_inserted=%d jobs_skipped=%d jobs_existing=%d",
		stats.ItemsSeen,
		stats.ItemsUpserted,
		stats.JobsInserted,
		stats.JobsSkipped,
		stats.JobsExisting,
	)
}
