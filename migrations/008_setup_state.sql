CREATE TABLE setup_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    token_hash TEXT,
    expires_at INTEGER,
    claimed_at INTEGER,
    created_at INTEGER NOT NULL
);
