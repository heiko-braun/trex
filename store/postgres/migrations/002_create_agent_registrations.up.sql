CREATE TABLE IF NOT EXISTS agent_registrations (
    agent_id      TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    task_queue    TEXT NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
