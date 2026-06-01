PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS localpager_items (
  id INTEGER PRIMARY KEY,
  source TEXT,
  type TEXT,
  ref TEXT,
  source_kind TEXT NOT NULL,
  source_ref TEXT NOT NULL,
  source_url TEXT,
  title TEXT,
  state TEXT,
  author TEXT,
  latest_content_hash TEXT,
  metadata_json TEXT,
  source_updated_at TEXT,
  first_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS localpager_jobs (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES localpager_items(id),
  job_kind TEXT NOT NULL,
  processor_name TEXT NOT NULL,
  processor_version TEXT NOT NULL,
  content_hash TEXT NOT NULL,
  priority INTEGER NOT NULL DEFAULT 100,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  leased_until TEXT,
  run_after TEXT,
  notification_suppression_reason TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS localpager_results (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES localpager_items(id),
  job_id INTEGER NOT NULL REFERENCES localpager_jobs(id),
  job_kind TEXT NOT NULL,
  output_json TEXT NOT NULL,
  interest TEXT,
  topics_json TEXT,
  session_path TEXT,
  prompt_path TEXT,
  model TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS localpager_notifications (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES localpager_items(id),
  result_id INTEGER NOT NULL REFERENCES localpager_results(id),
  job_id INTEGER NOT NULL REFERENCES localpager_jobs(id),
  notification_kind TEXT NOT NULL,
  destination_kind TEXT NOT NULL,
  destination_ref TEXT NOT NULL,
  message_key TEXT NOT NULL,
  message_body TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  suppression_reason TEXT,
  attempts INTEGER NOT NULL DEFAULT 0,
  sent_at TEXT,
  external_message_id TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS localpager_items_source_idx
ON localpager_items(source_kind, source_ref);

CREATE UNIQUE INDEX IF NOT EXISTS uq_localpager_items_source
ON localpager_items(source_kind, source_ref);

CREATE INDEX IF NOT EXISTS localpager_items_seen_idx
ON localpager_items(source_kind, last_seen_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_localpager_items_generic
ON localpager_items(source, type, ref);

CREATE INDEX IF NOT EXISTS localpager_items_generic_seen_idx
ON localpager_items(source, type, source_updated_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_localpager_jobs_content
ON localpager_jobs(item_id, processor_name, processor_version, content_hash);

CREATE INDEX IF NOT EXISTS localpager_jobs_pending_idx
ON localpager_jobs(status, run_after, priority, created_at);

CREATE INDEX IF NOT EXISTS localpager_jobs_lease_idx
ON localpager_jobs(status, leased_until);

CREATE INDEX IF NOT EXISTS localpager_results_item_idx
ON localpager_results(item_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_localpager_notifications_message
ON localpager_notifications(message_key, destination_kind, destination_ref);

CREATE INDEX IF NOT EXISTS localpager_notifications_pending_idx
ON localpager_notifications(status, created_at);

CREATE INDEX IF NOT EXISTS localpager_notifications_item_idx
ON localpager_notifications(item_id, notification_kind);

CREATE TABLE IF NOT EXISTS localpager_watchers (
  id INTEGER PRIMARY KEY,
  source TEXT NOT NULL,
  name TEXT NOT NULL,
  cursor TEXT,
  last_run_at TEXT,
  last_success_at TEXT,
  last_error TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_localpager_watchers
ON localpager_watchers(source, name);
