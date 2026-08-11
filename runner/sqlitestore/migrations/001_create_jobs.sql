-- +goose Up
CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    payload         TEXT NOT NULL,
    state           TEXT NOT NULL,
    attempts        INTEGER NOT NULL DEFAULT 0,
    idempotency_key TEXT NOT NULL DEFAULT '',
    enqueued_at     TEXT NOT NULL,
    next_run_at     TEXT NOT NULL,
    last_error      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_jobs_state_next_run_at ON jobs (state, next_run_at);

-- +goose Down
DROP TABLE jobs;
