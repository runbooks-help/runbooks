CREATE TABLE IF NOT EXISTS invites (
    id         TEXT PRIMARY KEY, -- hash of the one-time token
    user_id    TEXT,             -- NULL: new-user invite; set: re-enrolment
    role       TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    used_at    BIGINT            -- NULL until consumed
);
