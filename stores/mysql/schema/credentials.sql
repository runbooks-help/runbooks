CREATE TABLE IF NOT EXISTS credentials (
    id            VARCHAR(64) NOT NULL PRIMARY KEY,
    user_id       VARCHAR(64) NOT NULL,
    credential_id VARBINARY(1023) NOT NULL,
    public_key    BLOB NOT NULL,
    sign_count    BIGINT NOT NULL DEFAULT 0,
    transports    VARCHAR(255) NULL,      -- comma-separated
    aaguid        VARBINARY(255) NULL,
    label         VARCHAR(255) NOT NULL DEFAULT '',
    created_at    BIGINT NOT NULL,
    last_used_at  BIGINT NULL,            -- NULL until first use
    UNIQUE KEY credentials_credential_id (credential_id),
    KEY credentials_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
