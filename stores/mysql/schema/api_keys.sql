CREATE TABLE IF NOT EXISTS api_keys (
    id           VARCHAR(64) NOT NULL PRIMARY KEY, -- hash of the opaque read-scoped key
    user_id      VARCHAR(64) NOT NULL,
    label        VARCHAR(128) NOT NULL,
    created_by   VARCHAR(64) NOT NULL,
    created_at   BIGINT NOT NULL,
    last_used_at BIGINT NULL,                      -- NULL until first use
    revoked_at   BIGINT NULL,                      -- NULL while active
    KEY api_keys_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
