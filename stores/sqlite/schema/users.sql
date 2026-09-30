CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    email        TEXT UNIQUE,       -- nullable; only needed for commit attribution
    display_name TEXT NOT NULL,
    role         TEXT NOT NULL,     -- 'admin' | 'member'
    created_at   INTEGER NOT NULL,  -- Unix seconds (UTC)
    disabled_at  INTEGER            -- NULL while enabled
);
