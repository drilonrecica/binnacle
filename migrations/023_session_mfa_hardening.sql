-- TOTP replay protection: the last accepted 30-second step is stored per user
-- and any step at or below it is refused, so a code works exactly once.
ALTER TABLE users ADD COLUMN totp_last_step INTEGER NOT NULL DEFAULT 0;
-- Session fingerprints were stored but never checked. Binding a session to a
-- user agent or network prefix signs people out on browser updates and network
-- changes without stopping a stolen cookie, so the columns are dropped instead.
ALTER TABLE sessions DROP COLUMN user_agent_hash;
ALTER TABLE sessions DROP COLUMN ip_prefix_hash;
