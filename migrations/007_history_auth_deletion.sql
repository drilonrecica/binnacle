ALTER TABLE resource_samples_10s ADD COLUMN block_read_bps REAL;
ALTER TABLE resource_samples_10s ADD COLUMN block_write_bps REAL;
ALTER TABLE resource_samples_10s ADD COLUMN status TEXT;

ALTER TABLE host_rollups_1m ADD COLUMN memory_avg REAL;
ALTER TABLE host_rollups_1m ADD COLUMN memory_min REAL;
ALTER TABLE host_rollups_1m ADD COLUMN memory_max REAL;
ALTER TABLE host_rollups_1m ADD COLUMN network_rx_avg REAL;
ALTER TABLE host_rollups_1m ADD COLUMN network_tx_avg REAL;
ALTER TABLE host_rollups_15m ADD COLUMN memory_avg REAL;
ALTER TABLE host_rollups_15m ADD COLUMN network_rx_avg REAL;
ALTER TABLE host_rollups_15m ADD COLUMN network_tx_avg REAL;
ALTER TABLE host_rollups_1h ADD COLUMN memory_avg REAL;
ALTER TABLE host_rollups_1h ADD COLUMN network_rx_avg REAL;
ALTER TABLE host_rollups_1h ADD COLUMN network_tx_avg REAL;

ALTER TABLE resource_rollups_1m ADD COLUMN memory_avg REAL;
ALTER TABLE resource_rollups_1m ADD COLUMN network_rx_avg REAL;
ALTER TABLE resource_rollups_1m ADD COLUMN network_tx_avg REAL;
ALTER TABLE resource_rollups_1m ADD COLUMN block_read_avg REAL;
ALTER TABLE resource_rollups_1m ADD COLUMN block_write_avg REAL;
ALTER TABLE resource_rollups_15m ADD COLUMN memory_avg REAL;
ALTER TABLE resource_rollups_15m ADD COLUMN network_rx_avg REAL;
ALTER TABLE resource_rollups_15m ADD COLUMN network_tx_avg REAL;
ALTER TABLE resource_rollups_15m ADD COLUMN block_read_avg REAL;
ALTER TABLE resource_rollups_15m ADD COLUMN block_write_avg REAL;
ALTER TABLE resource_rollups_1h ADD COLUMN memory_avg REAL;
ALTER TABLE resource_rollups_1h ADD COLUMN network_rx_avg REAL;
ALTER TABLE resource_rollups_1h ADD COLUMN network_tx_avg REAL;
ALTER TABLE resource_rollups_1h ADD COLUMN block_read_avg REAL;
ALTER TABLE resource_rollups_1h ADD COLUMN block_write_avg REAL;

ALTER TABLE sessions ADD COLUMN user_agent_hash TEXT;
ALTER TABLE sessions ADD COLUMN ip_prefix_hash TEXT;
ALTER TABLE sessions ADD COLUMN csrf_hash TEXT;
CREATE INDEX sessions_user_active ON sessions(user_id, revoked_at, expires_at);

CREATE TABLE history_deletion_previews (
    id_hash TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    resource_id TEXT,
    before_ts INTEGER,
    fence_ts INTEGER NOT NULL,
    confirmation TEXT NOT NULL,
    summary_json TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    used_at INTEGER
);
CREATE TABLE history_deletion_jobs (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    resource_id TEXT,
    before_ts INTEGER,
    fence_ts INTEGER NOT NULL,
    confirmation TEXT NOT NULL,
    state TEXT NOT NULL,
    requested_by TEXT,
    requested_at INTEGER NOT NULL,
    started_at INTEGER,
    finished_at INTEGER,
    total_rows INTEGER NOT NULL,
    deleted_rows INTEGER NOT NULL DEFAULT 0,
    current_table TEXT,
    error_message TEXT
);
CREATE UNIQUE INDEX one_active_history_deletion ON history_deletion_jobs((1)) WHERE state IN ('queued','running','cancelling');
