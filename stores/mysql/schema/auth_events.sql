CREATE TABLE IF NOT EXISTS auth_events (
    id             BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    at             BIGINT NOT NULL,
    actor_user_id  VARCHAR(64) NULL,
    action         VARCHAR(64) NOT NULL,
    target_user_id VARCHAR(64) NULL,
    detail         VARCHAR(512) NULL,
    ip             VARCHAR(64) NULL,
    user_agent     VARCHAR(512) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
