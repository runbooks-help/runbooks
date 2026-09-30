CREATE TABLE IF NOT EXISTS webauthn_challenges (
    id         TEXT PRIMARY KEY, -- opaque cookie id
    kind       TEXT NOT NULL,    -- 'registration' | 'login'
    data       BYTEA NOT NULL,
    expires_at BIGINT NOT NULL
);
