CREATE TABLE IF NOT EXISTS credentials (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    credential_id BLOB NOT NULL UNIQUE, -- the authenticator's credential id
    public_key    BLOB NOT NULL,
    sign_count    INTEGER NOT NULL DEFAULT 0,
    transports    TEXT,                 -- comma-separated
    aaguid        BLOB,
    label         TEXT NOT NULL DEFAULT '',
    created_at    INTEGER NOT NULL,
    last_used_at  INTEGER               -- NULL until first use
);

CREATE INDEX IF NOT EXISTS credentials_user_id ON credentials (user_id);
