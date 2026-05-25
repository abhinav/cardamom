-- Schema used by sqlc for type inference. Must match the live DB after
-- migrations apply (see store.go `migrations`). Not executed at runtime.

CREATE TABLE issues (
    id        TEXT PRIMARY KEY,
    title     TEXT NOT NULL,
    type      TEXT NOT NULL,
    status    TEXT NOT NULL,
    priority  INTEGER NOT NULL,
    agent     TEXT,
    assignee  TEXT,
    created   INTEGER NOT NULL,
    updated   INTEGER NOT NULL,
    closed    INTEGER
);

CREATE TABLE deps (
    child_id  TEXT NOT NULL,
    parent_id TEXT NOT NULL,
    PRIMARY KEY (child_id, parent_id)
);
