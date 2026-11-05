-- SPDX-License-Identifier: AGPL-3.0-only
-- STO-7: the raw sample tables are keyed (entity, ts), so a range on ts alone
-- (sparklines over every resource, rollup windows, retention and history
-- deletion) scanned the whole table. These indexes serve those ranges;
-- collector_state_events had no index for the gap classifier's lookups.

CREATE INDEX resource_samples_10s_ts ON resource_samples_10s(ts);
CREATE INDEX host_samples_10s_ts ON host_samples_10s(ts);
CREATE INDEX container_instance_samples_10s_ts ON container_instance_samples_10s(ts);
CREATE INDEX collector_state_events_collector_ts ON collector_state_events(collector_name, ts);
