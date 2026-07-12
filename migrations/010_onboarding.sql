CREATE TABLE onboarding_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    exposure_mode TEXT,
    retention_preset TEXT,
    diagnostics_json TEXT,
    completed_at INTEGER,
    checklist_dismissed_at INTEGER,
    updated_at INTEGER NOT NULL
);
