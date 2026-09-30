CREATE TABLE IF NOT EXISTS invites (
    id         VARCHAR(64) NOT NULL PRIMARY KEY, -- hash of the one-time token
    user_id    VARCHAR(64) NULL,                 -- NULL: new-user invite; set: re-enrolment
    role       VARCHAR(16) NOT NULL,
    created_by VARCHAR(64) NOT NULL,
    created_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL,
    used_at    BIGINT NULL                       -- NULL until consumed
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
