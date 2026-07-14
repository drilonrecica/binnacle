ALTER TABLE users ADD COLUMN totp_enabled INTEGER NOT NULL DEFAULT 0 CHECK(totp_enabled IN (0,1));
ALTER TABLE users ADD COLUMN totp_secret_key TEXT;
ALTER TABLE users ADD COLUMN mfa_changed_at INTEGER;
CREATE TABLE recovery_codes (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    used_at INTEGER,
    PRIMARY KEY(user_id, code_hash)
);
ALTER TABLE sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'local' CHECK(auth_method IN ('local','proxy'));
ALTER TABLE sessions ADD COLUMN auth_subject TEXT;
CREATE INDEX sessions_auth_provenance ON sessions(auth_method, auth_subject, created_at);
