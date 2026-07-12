-- Broaden host telemetry storage to match the normative alpha schema.
ALTER TABLE host_samples_10s ADD COLUMN cpu_user_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_system_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_iowait_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_steal_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_5 REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_15 REAL;
ALTER TABLE host_samples_10s ADD COLUMN memory_available_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN memory_total_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN memory_used_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN memory_cached_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN memory_buffers_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN swap_used_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN swap_total_bytes INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN swap_used_pct REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_read_bps REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_write_bps REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_read_iops REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_write_iops REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_rx_packets_ps REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_tx_packets_ps REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_rx_errors_delta INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN network_tx_errors_delta INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN network_rx_drops_delta INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN network_tx_drops_delta INTEGER;

CREATE TABLE IF NOT EXISTS filesystem_samples_1m (
  ts INTEGER NOT NULL,
  host_id TEXT NOT NULL,
  mount_key TEXT NOT NULL,
  mount_point TEXT NOT NULL,
  fs_type TEXT NULL,
  total_bytes INTEGER NULL,
  used_bytes INTEGER NULL,
  available_bytes INTEGER NULL,
  used_pct REAL NULL,
  inodes_total INTEGER NULL,
  inodes_used INTEGER NULL,
  inodes_used_pct REAL NULL,
  PRIMARY KEY(host_id, mount_key, ts)
);

CREATE TABLE IF NOT EXISTS network_interface_samples_1m (
  ts INTEGER NOT NULL,
  host_id TEXT NOT NULL,
  interface_name TEXT NOT NULL,
  rx_bps REAL NULL,
  tx_bps REAL NULL,
  rx_packets_ps REAL NULL,
  tx_packets_ps REAL NULL,
  rx_errors_delta INTEGER NULL,
  tx_errors_delta INTEGER NULL,
  rx_drops_delta INTEGER NULL,
  tx_drops_delta INTEGER NULL,
  PRIMARY KEY(host_id, interface_name, ts)
);
