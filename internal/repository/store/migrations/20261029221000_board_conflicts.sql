-- +goose Up
CREATE TABLE board_conflicts (
    id TEXT NOT NULL PRIMARY KEY,
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT 'field' CHECK (category IN ('field', 'relationships', 'execution')),
    issue_uid BLOB REFERENCES issues(uid) ON DELETE CASCADE,
    related_uid BLOB REFERENCES issues(uid) ON DELETE CASCADE,
    member_key TEXT NOT NULL DEFAULT '',
    revision INTEGER NOT NULL CHECK (revision >= 0)
);
CREATE INDEX board_conflicts_by_board ON board_conflicts(board_id, id);

CREATE TABLE board_conflict_alternatives (
    conflict_id TEXT NOT NULL REFERENCES board_conflicts(id) ON DELETE CASCADE,
    alternative_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    waiting_present BOOLEAN NOT NULL DEFAULT FALSE,
    issue_uid BLOB REFERENCES issues(uid),
    related_uid BLOB REFERENCES issues(uid),
    member_key TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (conflict_id, alternative_id)
);

CREATE TABLE board_conflict_origins (
    conflict_id TEXT NOT NULL,
    alternative_id TEXT NOT NULL,
    writer_uid BLOB NOT NULL CHECK (length(writer_uid) = 16),
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
    PRIMARY KEY (conflict_id, alternative_id, writer_uid, sequence, ordinal),
    FOREIGN KEY (conflict_id, alternative_id)
        REFERENCES board_conflict_alternatives(conflict_id, alternative_id)
        ON DELETE CASCADE
);

CREATE TABLE board_conflict_issues (
    conflict_id TEXT NOT NULL REFERENCES board_conflicts(id) ON DELETE CASCADE,
    issue_uid BLOB NOT NULL REFERENCES issues(uid) ON DELETE CASCADE,
    field TEXT NOT NULL,
    PRIMARY KEY (conflict_id, issue_uid, field)
);
CREATE INDEX board_conflicts_by_issue ON board_conflict_issues(issue_uid, conflict_id);

-- +goose Down
DROP TABLE board_conflict_issues;
DROP TABLE board_conflict_origins;
DROP TABLE board_conflict_alternatives;
DROP TABLE board_conflicts;
