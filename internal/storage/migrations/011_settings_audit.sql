CREATE TABLE settings_audit (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    revision INTEGER NOT NULL,
    setting_key TEXT NOT NULL,
    previous_value_json TEXT,
    new_value_json TEXT NOT NULL,
    actor TEXT NOT NULL,
    changed_at INTEGER NOT NULL
);
CREATE INDEX settings_audit_revision ON settings_audit(revision);
INSERT OR IGNORE INTO application_metadata(key,value) VALUES('settings_revision','0');
