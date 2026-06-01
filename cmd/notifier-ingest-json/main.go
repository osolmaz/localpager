package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/osolmaz/localpager/internal/notifier"
)

func main() {
	var dbPath string
	var itemJSON string
	var jobType string
	var processorName string
	var processorVersion string
	var priority int
	var initialHydration bool
	var suppressionReason string
	var cutoverAt string

	flag.StringVar(&dbPath, "db", notifier.DefaultDBPath, "notifier SQLite database path")
	flag.StringVar(&itemJSON, "item-json", "", "generic notifier item JSON; reads stdin when empty")
	flag.StringVar(&jobType, "job-type", "", "job type; defaults to classify_<item type>")
	flag.StringVar(&processorName, "processor-name", notifier.DefaultProcessorName, "processor name")
	flag.StringVar(&processorVersion, "processor-version", notifier.DefaultProcessorVer, "processor version")
	flag.IntVar(&priority, "priority", 100, "job priority")
	flag.BoolVar(&initialHydration, "initial-hydration", false, "record item as already handled without classifying it")
	flag.StringVar(&suppressionReason, "suppression-reason", "", "optional notification suppression reason")
	flag.StringVar(&cutoverAt, "cutover-at", "", "RFC3339 timestamp; items updated before this are recorded as skipped")
	flag.Parse()

	if itemJSON == "" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatal(err)
		}
		itemJSON = string(data)
	}
	var item notifier.IngestItem
	if err := json.Unmarshal([]byte(itemJSON), &item); err != nil {
		log.Fatalf("invalid item JSON: %v", err)
	}
	var cutover *time.Time
	if cutoverAt != "" {
		parsed, err := time.Parse(time.RFC3339, cutoverAt)
		if err != nil {
			log.Fatalf("invalid --cutover-at: %v", err)
		}
		cutover = &parsed
	}

	ctx := context.Background()
	pool, err := notifier.NewPool(ctx, dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	result, err := notifier.Ingest(ctx, pool, item, notifier.IngestOptions{
		JobType:                       jobType,
		ProcessorName:                 processorName,
		ProcessorVersion:              processorVersion,
		Priority:                      priority,
		InitialHydration:              initialHydration,
		NotificationSuppressionReason: suppressionReason,
		CutoverAt:                     cutover,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "item_id=%d job_inserted=%t job_skipped=%t job_existing=%t suppressed=%t\n", result.ItemID, result.JobInserted, result.JobSkipped, result.JobExisting, result.Suppressed)
}
