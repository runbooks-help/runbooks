CREATE TABLE IF NOT EXISTS sessions (
    id           VARCHAR(64) NOT NULL PRIMARY KEY, -- hash of the opaque cookie value
    user_id      VARCHAR(64) NOT NULL,
    created_at   BIGINT NOT NULL,
    expires_at   BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL,
    user_agent   VARCHAR(512) NULL,
    ip           VARCHAR(64) NULL,
    KEY sessions_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
