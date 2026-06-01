PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS notifier_items (
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

CREATE TABLE IF NOT EXISTS notifier_jobs (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES notifier_items(id),
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

CREATE TABLE IF NOT EXISTS notifier_results (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES notifier_items(id),
  job_id INTEGER NOT NULL REFERENCES notifier_jobs(id),
  job_kind TEXT NOT NULL,
  output_json TEXT NOT NULL,
  interest TEXT,
  topics_json TEXT,
  session_path TEXT,
  prompt_path TEXT,
  model TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS notifier_notifications (
  id INTEGER PRIMARY KEY,
  item_id INTEGER NOT NULL REFERENCES notifier_items(id),
  result_id INTEGER NOT NULL REFERENCES notifier_results(id),
  job_id INTEGER NOT NULL REFERENCES notifier_jobs(id),
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

CREATE INDEX IF NOT EXISTS notifier_items_source_idx
ON notifier_items(source_kind, source_ref);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifier_items_source
ON notifier_items(source_kind, source_ref);

CREATE INDEX IF NOT EXISTS notifier_items_seen_idx
ON notifier_items(source_kind, last_seen_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifier_items_generic
ON notifier_items(source, type, ref);

CREATE INDEX IF NOT EXISTS notifier_items_generic_seen_idx
ON notifier_items(source, type, source_updated_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifier_jobs_content
ON notifier_jobs(item_id, processor_name, processor_version, content_hash);

CREATE INDEX IF NOT EXISTS notifier_jobs_pending_idx
ON notifier_jobs(status, run_after, priority, created_at);

CREATE INDEX IF NOT EXISTS notifier_jobs_lease_idx
ON notifier_jobs(status, leased_until);

CREATE INDEX IF NOT EXISTS notifier_results_item_idx
ON notifier_results(item_id, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifier_notifications_message
ON notifier_notifications(message_key, destination_kind, destination_ref);

CREATE INDEX IF NOT EXISTS notifier_notifications_pending_idx
ON notifier_notifications(status, created_at);

CREATE INDEX IF NOT EXISTS notifier_notifications_item_idx
ON notifier_notifications(item_id, notification_kind);

CREATE TABLE IF NOT EXISTS notifier_watchers (
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

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifier_watchers
ON notifier_watchers(source, name);
