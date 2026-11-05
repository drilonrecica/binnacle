-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR 021 §6: retention for the tables that had none. These indexes serve the
-- new retention predicates and the paginated archived resource list; no data
-- is rewritten.

CREATE INDEX resources_archived ON resources(status, archived_at);
CREATE INDEX container_instances_destroyed ON container_instances(destroyed_at);
CREATE INDEX alert_evaluation_state_evaluated ON alert_evaluation_state(last_evaluated_at);
