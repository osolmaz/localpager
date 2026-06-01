package localpager

import "time"

type Item struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Source            *string    `gorm:"column:source;type:text;uniqueIndex:uq_localpager_items_generic,priority:1;index:idx_localpager_items_generic_seen,priority:1"`
	Type              *string    `gorm:"column:type;type:text;uniqueIndex:uq_localpager_items_generic,priority:2;index:idx_localpager_items_generic_seen,priority:2"`
	Ref               *string    `gorm:"column:ref;type:text;uniqueIndex:uq_localpager_items_generic,priority:3"`
	SourceKind        string     `gorm:"column:source_kind;type:text;not null;uniqueIndex:uq_localpager_items_source,priority:1;index:idx_localpager_items_seen,priority:1"`
	SourceRef         string     `gorm:"column:source_ref;type:text;not null;uniqueIndex:uq_localpager_items_source,priority:2"`
	SourceURL         *string    `gorm:"column:source_url;type:text"`
	Title             *string    `gorm:"column:title;type:text"`
	State             *string    `gorm:"column:state;type:text"`
	Author            *string    `gorm:"column:author;type:text"`
	LatestContentHash *string    `gorm:"column:latest_content_hash;type:text"`
	MetadataJSON      *string    `gorm:"column:metadata_json;type:text"`
	SourceUpdatedAt   *time.Time `gorm:"column:source_updated_at;index:idx_localpager_items_generic_seen,priority:3"`
	FirstSeenAt       time.Time  `gorm:"column:first_seen_at;not null;default:CURRENT_TIMESTAMP"`
	LastSeenAt        time.Time  `gorm:"column:last_seen_at;not null;default:CURRENT_TIMESTAMP;index:idx_localpager_items_seen,priority:2"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`
}

func (Item) TableName() string { return "localpager_items" }

type Job struct {
	ID                            int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ItemID                        int64      `gorm:"column:item_id;not null;uniqueIndex:uq_localpager_jobs_content,priority:1"`
	JobKind                       string     `gorm:"column:job_kind;type:text;not null"`
	ProcessorName                 string     `gorm:"column:processor_name;type:text;not null;uniqueIndex:uq_localpager_jobs_content,priority:2"`
	ProcessorVersion              string     `gorm:"column:processor_version;type:text;not null;uniqueIndex:uq_localpager_jobs_content,priority:3"`
	ContentHash                   string     `gorm:"column:content_hash;type:text;not null;uniqueIndex:uq_localpager_jobs_content,priority:4"`
	Priority                      int        `gorm:"column:priority;not null;default:100;index:idx_localpager_jobs_pending,priority:3"`
	Status                        string     `gorm:"column:status;type:text;not null;default:pending;index:idx_localpager_jobs_pending,priority:1;index:idx_localpager_jobs_lease,priority:1"`
	Attempts                      int        `gorm:"column:attempts;not null;default:0"`
	LeasedUntil                   *time.Time `gorm:"column:leased_until;index:idx_localpager_jobs_lease,priority:2"`
	RunAfter                      *time.Time `gorm:"column:run_after;index:idx_localpager_jobs_pending,priority:2"`
	NotificationSuppressionReason *string    `gorm:"column:notification_suppression_reason;type:text"`
	LastError                     *string    `gorm:"column:last_error;type:text"`
	CreatedAt                     time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;index:idx_localpager_jobs_pending,priority:4"`
	UpdatedAt                     time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`

	Item Item `gorm:"foreignKey:ItemID"`
}

func (Job) TableName() string { return "localpager_jobs" }

type Result struct {
	ID          int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ItemID      int64     `gorm:"column:item_id;not null;index:idx_localpager_results_item,priority:1"`
	JobID       int64     `gorm:"column:job_id;not null"`
	JobKind     string    `gorm:"column:job_kind;type:text;not null"`
	OutputJSON  string    `gorm:"column:output_json;type:text;not null"`
	Interest    *string   `gorm:"column:interest;type:text"`
	TopicsJSON  *string   `gorm:"column:topics_json;type:text"`
	SessionPath *string   `gorm:"column:session_path;type:text"`
	PromptPath  *string   `gorm:"column:prompt_path;type:text"`
	Model       *string   `gorm:"column:model;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;index:idx_localpager_results_item,priority:2"`

	Item Item `gorm:"foreignKey:ItemID"`
	Job  Job  `gorm:"foreignKey:JobID"`
}

func (Result) TableName() string { return "localpager_results" }

type Notification struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ItemID            int64      `gorm:"column:item_id;not null;index:idx_localpager_notifications_item,priority:1"`
	ResultID          int64      `gorm:"column:result_id;not null"`
	JobID             int64      `gorm:"column:job_id;not null"`
	NotificationKind  string     `gorm:"column:notification_kind;type:text;not null;index:idx_localpager_notifications_item,priority:2"`
	DestinationKind   string     `gorm:"column:destination_kind;type:text;not null;uniqueIndex:uq_localpager_notifications_message,priority:2"`
	DestinationRef    string     `gorm:"column:destination_ref;type:text;not null;uniqueIndex:uq_localpager_notifications_message,priority:3"`
	MessageKey        string     `gorm:"column:message_key;type:text;not null;uniqueIndex:uq_localpager_notifications_message,priority:1"`
	MessageBody       string     `gorm:"column:message_body;type:text;not null"`
	Status            string     `gorm:"column:status;type:text;not null;default:pending;index:idx_localpager_notifications_pending,priority:1"`
	SuppressionReason *string    `gorm:"column:suppression_reason;type:text"`
	Attempts          int        `gorm:"column:attempts;not null;default:0"`
	SentAt            *time.Time `gorm:"column:sent_at"`
	ExternalMessageID *string    `gorm:"column:external_message_id;type:text"`
	LastError         *string    `gorm:"column:last_error;type:text"`
	CreatedAt         time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP;index:idx_localpager_notifications_pending,priority:2"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`

	Item   Item   `gorm:"foreignKey:ItemID"`
	Result Result `gorm:"foreignKey:ResultID"`
	Job    Job    `gorm:"foreignKey:JobID"`
}

func (Notification) TableName() string { return "localpager_notifications" }

type WatcherState struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Source        string     `gorm:"column:source;type:text;not null;uniqueIndex:uq_localpager_watchers,priority:1"`
	Name          string     `gorm:"column:name;type:text;not null;uniqueIndex:uq_localpager_watchers,priority:2"`
	Cursor        *string    `gorm:"column:cursor;type:text"`
	LastRunAt     *time.Time `gorm:"column:last_run_at"`
	LastSuccessAt *time.Time `gorm:"column:last_success_at"`
	LastError     *string    `gorm:"column:last_error;type:text"`
	CreatedAt     time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`
}

func (WatcherState) TableName() string { return "localpager_watchers" }

func autoMigrateModels() []any {
	return []any{
		&Item{},
		&Job{},
		&Result{},
		&Notification{},
		&WatcherState{},
	}
}
