CREATE TABLE IF NOT EXISTS users (
    id           VARCHAR(64) NOT NULL PRIMARY KEY,
    email        VARCHAR(255) NULL,       -- only needed for commit attribution
    display_name VARCHAR(255) NOT NULL,
    role         VARCHAR(16) NOT NULL,    -- 'admin' | 'member'
    created_at   BIGINT NOT NULL,         -- Unix seconds (UTC)
    disabled_at  BIGINT NULL,             -- NULL while enabled
    UNIQUE KEY users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
