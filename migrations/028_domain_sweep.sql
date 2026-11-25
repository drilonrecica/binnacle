-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR 028: the domain sweep. Public domains the operator owns, probed on an
-- interval independent of Docker resources. sweep_state holds the latest
-- result per domain; a fingerprint change between two runs is written to the
-- events table as a domain_change event.

CREATE TABLE sweep_domains (
  id TEXT PRIMARY KEY, domain TEXT UNIQUE NOT NULL,
  interval_seconds INTEGER NOT NULL CHECK(interval_seconds BETWEEN 600 AND 86400),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),
  created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE sweep_state (
  domain_id TEXT PRIMARY KEY REFERENCES sweep_domains(id) ON DELETE CASCADE,
  checked_at INTEGER NOT NULL, next_run_at INTEGER NOT NULL,
  http_status INTEGER, failure_code TEXT, cert_not_after INTEGER, cert_days_left INTEGER,
  ns_present INTEGER NOT NULL DEFAULT 0 CHECK(ns_present IN (0,1)),
  resolves INTEGER NOT NULL DEFAULT 0 CHECK(resolves IN (0,1)),
  spf TEXT, dmarc_policy TEXT,
  hsts INTEGER NOT NULL DEFAULT 0 CHECK(hsts IN (0,1)), csp INTEGER NOT NULL DEFAULT 0 CHECK(csp IN (0,1)),
  fingerprint TEXT NOT NULL DEFAULT '', details_json TEXT NOT NULL DEFAULT '{}',
  consecutive_failures INTEGER NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL
);
CREATE INDEX sweep_state_due ON sweep_state(next_run_at);
