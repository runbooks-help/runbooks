CREATE TABLE IF NOT EXISTS credentials (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    credential_id BYTEA NOT NULL UNIQUE, -- the authenticator's credential id
    public_key    BYTEA NOT NULL,
    sign_count    BIGINT NOT NULL DEFAULT 0,
    transports    TEXT,                  -- comma-separated
    aaguid        BYTEA,
    label         TEXT NOT NULL DEFAULT '',
    created_at    BIGINT NOT NULL,
    last_used_at  BIGINT                 -- NULL until first use
);

CREATE INDEX IF NOT EXISTS credentials_user_id ON credentials (user_id);
