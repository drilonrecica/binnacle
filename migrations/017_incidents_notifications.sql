CREATE TABLE incidents (
  id TEXT PRIMARY KEY,
  group_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('open','resolved')),
  severity TEXT NOT NULL CHECK(severity IN ('warning','critical')),
  target_type TEXT NOT NULL,
  target_id TEXT NOT NULL,
  title TEXT NOT NULL,
  opened_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  resolved_at INTEGER,
  version INTEGER NOT NULL DEFAULT 1,
  next_reminder_at INTEGER
);
CREATE UNIQUE INDEX incidents_one_open_group ON incidents(group_key) WHERE status='open';
CREATE INDEX incidents_list ON incidents(status, severity, opened_at DESC);
CREATE INDEX incidents_retention ON incidents(resolved_at);
CREATE INDEX incidents_reminders ON incidents(next_reminder_at) WHERE status='open';

CREATE TABLE incident_alerts (
  incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
  alert_id TEXT NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
  joined_at INTEGER NOT NULL,
  resolved_at INTEGER,
  PRIMARY KEY(incident_id, alert_id),
  UNIQUE(alert_id)
);
CREATE INDEX incident_alerts_incident ON incident_alerts(incident_id, resolved_at);

CREATE TABLE notification_channels (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
  kind TEXT NOT NULL CHECK(kind IN ('webhook','smtp')),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
  minimum_severity TEXT NOT NULL DEFAULT 'warning' CHECK(minimum_severity IN ('warning','critical')),
  notify_resolved INTEGER NOT NULL DEFAULT 1 CHECK(notify_resolved IN (0,1)),
  config_json TEXT NOT NULL DEFAULT '{}',
  secret_ref TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  deleted_at INTEGER
);
CREATE INDEX notification_channels_active ON notification_channels(enabled, deleted_at);
CREATE TRIGGER notification_channels_limit
BEFORE INSERT ON notification_channels
WHEN (SELECT COUNT(*) FROM notification_channels WHERE deleted_at IS NULL) >= 32
BEGIN
  SELECT RAISE(ABORT, 'notification channel limit reached');
END;

CREATE TABLE notification_deliveries (
  id TEXT PRIMARY KEY,
  channel_id TEXT NOT NULL REFERENCES notification_channels(id),
  incident_id TEXT REFERENCES incidents(id) ON DELETE SET NULL,
  event_type TEXT NOT NULL CHECK(event_type IN ('opened','updated','reminder','resolved','test')),
  payload_json TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK(status IN ('pending','in_progress','succeeded','permanent_failure','cancelled')),
  attempt_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at INTEGER,
  started_at INTEGER,
  completed_at INTEGER,
  failure_code TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE INDEX notification_deliveries_due ON notification_deliveries(status, next_attempt_at);
CREATE INDEX notification_deliveries_list ON notification_deliveries(created_at DESC);
CREATE INDEX notification_deliveries_incident ON notification_deliveries(incident_id, created_at DESC);
