package store

const schemaSQL = `
PRAGMA journal_mode=WAL;
PRAGMA busy_timeout=5000;

CREATE TABLE IF NOT EXISTS jobs (
    id             TEXT PRIMARY KEY,
    origin_url     TEXT NOT NULL,
    max_depth      INTEGER NOT NULL DEFAULT 3,
    rate_limit     INTEGER NOT NULL DEFAULT 2,
    max_queue_size INTEGER NOT NULL DEFAULT 10000,
    status         TEXT NOT NULL DEFAULT 'pending',
    pages_crawled  INTEGER NOT NULL DEFAULT 0,
    pages_failed   INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    finished_at    TEXT,
    error          TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);
`
