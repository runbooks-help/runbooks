CREATE TABLE IF NOT EXISTS auth_events (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    at             INTEGER NOT NULL,
    actor_user_id  TEXT,
    action         TEXT NOT NULL,
    target_user_id TEXT,
    detail         TEXT,
    ip             TEXT,
    user_agent     TEXT
);
