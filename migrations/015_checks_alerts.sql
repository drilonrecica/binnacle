CREATE TABLE health_checks (
  id TEXT PRIMARY KEY, resource_id TEXT NOT NULL REFERENCES resources(id), name TEXT NOT NULL,
  url TEXT NOT NULL, method TEXT NOT NULL CHECK(method IN ('GET','HEAD')),
  interval_seconds INTEGER NOT NULL CHECK(interval_seconds BETWEEN 10 AND 3600),
  timeout_seconds INTEGER NOT NULL CHECK(timeout_seconds BETWEEN 1 AND 30),
  expected_status_min INTEGER NOT NULL CHECK(expected_status_min BETWEEN 100 AND 599),
  expected_status_max INTEGER NOT NULL CHECK(expected_status_max BETWEEN 100 AND 599 AND expected_status_max >= expected_status_min),
  body_substring TEXT CHECK(body_substring IS NULL OR length(body_substring) <= 256),
  required INTEGER NOT NULL DEFAULT 1 CHECK(required IN (0,1)), enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE INDEX health_checks_resource ON health_checks(resource_id);
CREATE TABLE health_check_state (
  check_id TEXT PRIMARY KEY REFERENCES health_checks(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK(status IN ('unknown','success','failure','running')),
  failure_code TEXT, latency_ms INTEGER, checked_at INTEGER, consecutive_successes INTEGER NOT NULL DEFAULT 0,
  consecutive_failures INTEGER NOT NULL DEFAULT 0, next_run_at INTEGER, updated_at INTEGER NOT NULL
);
CREATE INDEX health_check_state_due ON health_check_state(next_run_at);

CREATE TABLE alert_rules (
  id TEXT PRIMARY KEY, family TEXT NOT NULL, name TEXT NOT NULL, built_in INTEGER NOT NULL DEFAULT 0 CHECK(built_in IN (0,1)),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)), severity TEXT NOT NULL CHECK(severity IN ('warning','critical')),
  scope_type TEXT NOT NULL CHECK(scope_type IN ('global','host','filesystem','project','resource','check')),
  scope_id TEXT NOT NULL DEFAULT '', threshold REAL, recovery_threshold REAL,
  trigger_seconds INTEGER NOT NULL, recovery_seconds INTEGER NOT NULL, window_seconds INTEGER NOT NULL DEFAULT 0,
  cooldown_seconds INTEGER NOT NULL DEFAULT 300, repeat_seconds INTEGER NOT NULL DEFAULT 7200,
  suppress_during_deployment INTEGER NOT NULL DEFAULT 0 CHECK(suppress_during_deployment IN (0,1)),
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, UNIQUE(family, scope_type, scope_id)
);
CREATE INDEX alert_rules_lookup ON alert_rules(family, enabled, scope_type, scope_id);
CREATE TABLE alert_evaluation_state (
  dedup_key TEXT PRIMARY KEY, rule_id TEXT NOT NULL REFERENCES alert_rules(id), target_type TEXT NOT NULL, target_id TEXT NOT NULL,
  phase TEXT NOT NULL CHECK(phase IN ('healthy','pending','firing','recovering')),
  phase_since INTEGER NOT NULL, last_evaluated_at INTEGER NOT NULL, last_notified_at INTEGER,
  cooldown_until INTEGER, observed_value REAL, details_json TEXT NOT NULL DEFAULT '{}'
);
CREATE TABLE alerts (
  id TEXT PRIMARY KEY, dedup_key TEXT NOT NULL, rule_id TEXT NOT NULL REFERENCES alert_rules(id), family TEXT NOT NULL,
  severity TEXT NOT NULL CHECK(severity IN ('warning','critical')), target_type TEXT NOT NULL, target_id TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('firing','resolved')), started_at INTEGER NOT NULL, resolved_at INTEGER,
  last_observed_at INTEGER NOT NULL, observed_value REAL, message TEXT NOT NULL, UNIQUE(dedup_key, started_at)
);
CREATE INDEX alerts_current ON alerts(status, severity, started_at);
CREATE INDEX alerts_retention ON alerts(resolved_at);
CREATE TABLE silences (
  id TEXT PRIMARY KEY, scope_type TEXT NOT NULL CHECK(scope_type IN ('server','project','resource','rule')),
  scope_id TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 500),
  starts_at INTEGER NOT NULL, ends_at INTEGER NOT NULL CHECK(ends_at > starts_at), created_by TEXT NOT NULL, created_at INTEGER NOT NULL
);
CREATE INDEX silences_active ON silences(starts_at, ends_at);
CREATE TABLE deployment_grace_periods (
  resource_id TEXT PRIMARY KEY REFERENCES resources(id) ON DELETE CASCADE, starts_at INTEGER NOT NULL,
  ends_at INTEGER NOT NULL CHECK(ends_at > starts_at), confidence TEXT NOT NULL CHECK(confidence IN ('confirmed','likely'))
);
