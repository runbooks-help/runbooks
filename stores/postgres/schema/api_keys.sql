CREATE TABLE IF NOT EXISTS api_keys (
    id           TEXT PRIMARY KEY, -- hash of the opaque read-scoped key
    user_id      TEXT NOT NULL,
    label        TEXT NOT NULL,
    created_by   TEXT NOT NULL,
    created_at   BIGINT NOT NULL,
    last_used_at BIGINT,           -- NULL until first use
    revoked_at   BIGINT            -- NULL while active
);

CREATE INDEX IF NOT EXISTS api_keys_user_id ON api_keys (user_id);
