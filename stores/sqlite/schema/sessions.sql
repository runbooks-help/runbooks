CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY, -- hash of the opaque cookie value
    user_id      TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    user_agent   TEXT,
    ip           TEXT
);

CREATE INDEX IF NOT EXISTS sessions_user_id ON sessions (user_id);
