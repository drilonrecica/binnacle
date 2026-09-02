-- The Overview, Host, and Activity screens replaced Watch, Server, and Events.
-- SQLite cannot alter CHECK constraints, so the table is rebuilt and stored
-- values are mapped to their replacements.
CREATE TABLE user_preferences_next (
    user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK(schema_version = 1),
    theme TEXT NOT NULL CHECK(theme IN ('system','dark','light')),
    density TEXT NOT NULL CHECK(density IN ('comfortable','compact')),
    pinned_resources_json TEXT NOT NULL DEFAULT '[]' CHECK(length(pinned_resources_json) <= 4096),
    landing_page TEXT NOT NULL CHECK(landing_page IN ('overview','resources','host','alerts','activity','logs')),
    chart_range TEXT NOT NULL CHECK(chart_range IN ('1h','6h','24h','7d','30d')),
    updated_at INTEGER NOT NULL
);
INSERT INTO user_preferences_next(user_id,schema_version,theme,density,pinned_resources_json,landing_page,chart_range,updated_at)
SELECT user_id,schema_version,theme,density,pinned_resources_json,
    CASE landing_page WHEN 'watch' THEN 'overview' WHEN 'server' THEN 'host' WHEN 'events' THEN 'activity' ELSE landing_page END,
    chart_range,updated_at
FROM user_preferences;
DROP TABLE user_preferences;
ALTER TABLE user_preferences_next RENAME TO user_preferences;
