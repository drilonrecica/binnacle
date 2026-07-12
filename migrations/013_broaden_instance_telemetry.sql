-- SPDX-License-Identifier: AGPL-3.0-only
-- Broaden container instance sample schema to match SPEC §23.7.
ALTER TABLE container_instance_samples_10s ADD COLUMN cpu_core_equiv REAL NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN memory_usage_bytes INTEGER NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN network_rx_bps REAL NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN network_tx_bps REAL NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN block_read_bps REAL NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN block_write_bps REAL NULL;
ALTER TABLE container_instance_samples_10s ADD COLUMN pids INTEGER NULL;
