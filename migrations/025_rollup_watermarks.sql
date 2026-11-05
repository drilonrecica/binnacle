-- SPDX-License-Identifier: AGPL-3.0-only
-- ADR 021 §4 and §5: rollups are incremental from a persisted watermark and
-- raw retention never runs ahead of the final 1h buckets. One row per rollup
-- table ('host_rollups_1h', ...) holds closed_until: buckets with
-- ts < closed_until are computed. The row 'raw_deleted_below' holds the raw
-- retention cutoff: raw rows below it may be gone, so no bucket below it is
-- ever recomputed. Absent rows mean the first pass after the upgrade still
-- has to run the one-off repair (ADR 021 §8).

CREATE TABLE rollup_watermarks (
    table_name TEXT PRIMARY KEY,
    closed_until INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
