CREATE TABLE IF NOT EXISTS webauthn_challenges (
    id         VARCHAR(64) NOT NULL PRIMARY KEY, -- opaque cookie id
    kind       VARCHAR(32) NOT NULL,             -- 'registration' | 'login'
    data       BLOB NOT NULL,
    expires_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
