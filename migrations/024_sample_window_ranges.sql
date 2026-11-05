-- SPDX-License-Identifier: AGPL-3.0-only
-- COL-9: a raw sample is the mean of the collector ticks of its 10 s window;
-- these columns keep the smallest and largest tick so rollup min/max see
-- bursts between two samples. NULL on rows written before this migration;
-- readers fall back to the sample value. disk_iops is read plus write IOPS.

ALTER TABLE host_samples_10s ADD COLUMN cpu_busy_pct_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_busy_pct_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_user_pct_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_user_pct_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_system_pct_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_system_pct_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_iowait_pct_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_iowait_pct_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_steal_pct_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN cpu_steal_pct_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_1_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_1_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_5_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_5_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_15_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN load_15_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN memory_used_bytes_min INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN memory_used_bytes_max INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN swap_used_bytes_min INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN swap_used_bytes_max INTEGER;
ALTER TABLE host_samples_10s ADD COLUMN network_rx_bps_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_rx_bps_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_tx_bps_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN network_tx_bps_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_read_bps_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_read_bps_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_write_bps_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_write_bps_max REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_iops_min REAL;
ALTER TABLE host_samples_10s ADD COLUMN disk_iops_max REAL;

ALTER TABLE resource_samples_10s ADD COLUMN cpu_host_pct_min REAL;
ALTER TABLE resource_samples_10s ADD COLUMN cpu_host_pct_max REAL;
ALTER TABLE resource_samples_10s ADD COLUMN memory_working_set_bytes_min INTEGER;
ALTER TABLE resource_samples_10s ADD COLUMN memory_working_set_bytes_max INTEGER;
ALTER TABLE resource_samples_10s ADD COLUMN network_rx_bps_min REAL;
ALTER TABLE resource_samples_10s ADD COLUMN network_rx_bps_max REAL;
ALTER TABLE resource_samples_10s ADD COLUMN network_tx_bps_min REAL;
ALTER TABLE resource_samples_10s ADD COLUMN network_tx_bps_max REAL;
ALTER TABLE resource_samples_10s ADD COLUMN block_read_bps_min REAL;
ALTER TABLE resource_samples_10s ADD COLUMN block_read_bps_max REAL;
ALTER TABLE resource_samples_10s ADD COLUMN block_write_bps_min REAL;
ALTER TABLE resource_samples_10s ADD COLUMN block_write_bps_max REAL;
