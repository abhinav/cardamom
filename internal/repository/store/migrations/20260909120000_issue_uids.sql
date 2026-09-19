-- +goose Up

-- Search is a projection of canonical issue records. Remove its triggers and
-- rows before replacing issue tables, then rebuild it from the migrated rows.
-- This keeps the search schema and UID rewrite in one migration transaction.
DROP TRIGGER issue_search_documents_fts_insert;
DROP TRIGGER issue_search_documents_fts_delete;
DROP TRIGGER issue_search_documents_fts_update;
DROP TRIGGER issues_search_insert;
DROP TRIGGER issues_search_update;
DROP TRIGGER issues_search_delete;
DROP TRIGGER issue_states_search_insert;
DROP TRIGGER issue_states_search_update;
DROP TRIGGER issue_states_search_delete;
DROP TRIGGER issue_results_search_insert;
DROP TRIGGER issue_results_search_update;
DROP TRIGGER issue_results_search_delete;
DROP TRIGGER issue_log_entries_search_insert;
DROP TRIGGER issue_log_entries_search_update;
DROP TRIGGER issue_log_entries_search_delete;
DROP TABLE issue_search_fts;
DROP TABLE issue_search_documents;

-- Issues retain their public ID as a store-wide selector, while a private UID
-- becomes the immutable row identity used by every issue-owned relation. The
-- replacement tables move the complete graph in this migration transaction;
-- a failed copy therefore leaves the preceding schema and data active.
CREATE TABLE issues_next (
    uid BLOB NOT NULL PRIMARY KEY CHECK (
        typeof(uid) = 'blob'
        AND length(uid) = 16
        AND uid <> zeroblob(16)
    ),
    id TEXT NOT NULL UNIQUE CHECK (
        substr(id, 1, 1) GLOB '[A-Za-z0-9]'
        AND id NOT GLOB '*[^A-Za-z0-9-]*'
    ),
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    title TEXT NOT NULL CHECK (length(trim(title)) > 0),
    kind TEXT NOT NULL CHECK (
        kind IN ('workstream', 'task', 'checkpoint', 'routine')
    ),
    lifecycle TEXT NOT NULL CHECK (
        lifecycle IN ('open', 'closed', 'cancelled')
    ),
    priority INTEGER NOT NULL CHECK (priority BETWEEN 0 AND 4),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    closed_at TIMESTAMP,
    waiting_reason TEXT,
    waiting_since TIMESTAMP,
    summary TEXT CHECK (
        summary IS NULL OR length(trim(summary)) > 0
    ),
    details TEXT CHECK (details IS NULL OR length(trim(details)) > 0),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    UNIQUE (board_id, uid),
    CHECK (
        (lifecycle = 'open' AND closed_at IS NULL)
        OR (lifecycle <> 'open' AND closed_at IS NOT NULL)
    ),
    CHECK (
        (waiting_reason IS NULL AND waiting_since IS NULL)
        OR (
            waiting_reason IS NOT NULL
            AND waiting_since IS NOT NULL
            AND lifecycle = 'open'
            AND length(trim(waiting_reason)) > 0
            AND instr(waiting_reason, char(9)) = 0
            AND instr(waiting_reason, char(10)) = 0
            AND instr(waiting_reason, char(13)) = 0
        )
    )
);

INSERT INTO issues_next (
    uid,
    id,
    board_id,
    title,
    kind,
    lifecycle,
    priority,
    created_at,
    updated_at,
    closed_at,
    waiting_reason,
    waiting_since,
    summary,
    details,
    revision
)
SELECT
    randomblob(16),
    id,
    board_id,
    title,
    kind,
    lifecycle,
    priority,
    created_at,
    updated_at,
    closed_at,
    waiting_reason,
    waiting_since,
    summary,
    details,
    revision
FROM issues;

CREATE TABLE active_claims_next (
    issue_uid BLOB NOT NULL PRIMARY KEY,
    board_id TEXT NOT NULL,
    actor TEXT NOT NULL CHECK (length(trim(actor)) > 0),
    started_at TIMESTAMP NOT NULL,
    started_revision INTEGER NOT NULL CHECK (started_revision > 0),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO active_claims_next (
    issue_uid,
    board_id,
    actor,
    started_at,
    started_revision
)
SELECT issue.uid, claim.board_id, claim.actor,
    claim.started_at, claim.started_revision
FROM active_claims AS claim
JOIN issues_next AS issue
    ON issue.board_id = claim.board_id
    AND issue.id = claim.issue_id;

CREATE TABLE issue_labels_next (
    board_id TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    label TEXT NOT NULL CHECK (length(trim(label)) > 0),
    PRIMARY KEY (issue_uid, label),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO issue_labels_next (board_id, issue_uid, label)
SELECT label.board_id, issue.uid, label.label
FROM issue_labels AS label
JOIN issues_next AS issue
    ON issue.board_id = label.board_id
    AND issue.id = label.issue_id;

CREATE TABLE dependencies_next (
    board_id TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    prerequisite_uid BLOB NOT NULL,
    PRIMARY KEY (issue_uid, prerequisite_uid),
    CHECK (issue_uid <> prerequisite_uid),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE,
    FOREIGN KEY (board_id, prerequisite_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO dependencies_next (board_id, issue_uid, prerequisite_uid)
SELECT dependency.board_id, issue.uid, prerequisite.uid
FROM dependencies AS dependency
JOIN issues_next AS issue
    ON issue.board_id = dependency.board_id
    AND issue.id = dependency.issue_id
JOIN issues_next AS prerequisite
    ON prerequisite.board_id = dependency.board_id
    AND prerequisite.id = dependency.prerequisite_id;

CREATE TABLE containment_next (
    board_id TEXT NOT NULL,
    child_uid BLOB NOT NULL PRIMARY KEY,
    parent_uid BLOB NOT NULL,
    CHECK (child_uid <> parent_uid),
    FOREIGN KEY (board_id, child_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE,
    FOREIGN KEY (board_id, parent_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO containment_next (board_id, child_uid, parent_uid)
SELECT relation.board_id, child.uid, parent.uid
FROM containment AS relation
JOIN issues_next AS child
    ON child.board_id = relation.board_id
    AND child.id = relation.child_id
JOIN issues_next AS parent
    ON parent.board_id = relation.board_id
    AND parent.id = relation.parent_id;

CREATE TABLE issue_external_keys_next (
    board_id TEXT NOT NULL,
    external_key TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    PRIMARY KEY (board_id, external_key),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO issue_external_keys_next (board_id, external_key, issue_uid)
SELECT key.board_id, key.external_key, issue.uid
FROM issue_external_keys AS key
JOIN issues_next AS issue
    ON issue.board_id = key.board_id
    AND issue.id = key.issue_id;

CREATE TABLE issue_results_next (
    issue_uid BLOB NOT NULL PRIMARY KEY,
    board_id TEXT NOT NULL,
    body TEXT NOT NULL CHECK (length(trim(body)) > 0),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO issue_results_next (issue_uid, board_id, body)
SELECT issue.uid, result.board_id, result.body
FROM issue_results AS result
JOIN issues_next AS issue
    ON issue.board_id = result.board_id
    AND issue.id = result.issue_id;

-- AUTOINCREMENT forbids reusing a sequence even after its row is deleted.
-- Rebuilding the log table derives a new sqlite_sequence entry only from
-- retained rows, so retain the predecessor's independent high-water mark.
CREATE TABLE issue_log_entries_sequence_guard (
    high_water INTEGER NOT NULL CHECK (high_water >= 0)
);

INSERT INTO issue_log_entries_sequence_guard (high_water)
SELECT coalesce((
    SELECT seq
    FROM sqlite_sequence
    WHERE name = 'issue_log_entries'
), 0);

CREATE TABLE issue_log_entries_next (
    local_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE CHECK (
        length(id) = 36
        AND substr(id, 1, 4) IN ('cmt_', 'log_')
        AND substr(id, 5) NOT GLOB '*[^0-9a-f]*'
    ),
    board_id TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('post', 'state_snapshot')),
    author TEXT CHECK (
        author IS NULL
        OR (length(author) > 0 AND author = trim(author))
    ),
    committer TEXT CHECK (
        committer IS NULL
        OR (length(committer) > 0 AND committer = trim(committer))
    ),
    body TEXT NOT NULL CHECK (length(body) > 0),
    created_at TIMESTAMP,
    next_action TEXT CHECK (
        next_action IS NULL OR trim(next_action) <> ''
    ),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE,
    UNIQUE (issue_uid, id),
    CHECK (
        kind <> 'post'
        OR (
            author IS NOT NULL
            AND committer IS NOT NULL
            AND created_at IS NOT NULL
        )
    )
);

INSERT INTO issue_log_entries_next (
    local_sequence,
    id,
    board_id,
    issue_uid,
    kind,
    author,
    committer,
    body,
    created_at,
    next_action
)
SELECT log.local_sequence, log.id, log.board_id, issue.uid,
    log.kind, log.author, log.committer, log.body,
    log.created_at, log.next_action
FROM issue_log_entries AS log
JOIN issues_next AS issue
    ON issue.board_id = log.board_id
    AND issue.id = log.issue_id;

CREATE TABLE issue_states_next (
    issue_uid BLOB NOT NULL PRIMARY KEY,
    board_id TEXT NOT NULL,
    body TEXT NOT NULL CHECK (length(trim(body)) > 0),
    author TEXT CHECK (
        author IS NULL
        OR (length(author) > 0 AND author = trim(author))
    ),
    updated_at TIMESTAMP,
    snapshot_log_entry_id TEXT,
    next_action TEXT CHECK (
        next_action IS NULL OR trim(next_action) <> ''
    ),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE,
    FOREIGN KEY (issue_uid, snapshot_log_entry_id)
        REFERENCES issue_log_entries_next(issue_uid, id) ON DELETE RESTRICT,
    CHECK (
        (author IS NULL AND updated_at IS NULL)
        OR (author IS NOT NULL AND updated_at IS NOT NULL)
    )
);

INSERT INTO issue_states_next (
    issue_uid,
    board_id,
    body,
    author,
    updated_at,
    snapshot_log_entry_id,
    next_action
)
SELECT issue.uid, state.board_id, state.body, state.author,
    state.updated_at, state.snapshot_log_entry_id, state.next_action
FROM issue_states AS state
JOIN issues_next AS issue
    ON issue.board_id = state.board_id
    AND issue.id = state.issue_id;

CREATE TABLE checkpoint_decisions_next (
    issue_uid BLOB NOT NULL PRIMARY KEY,
    board_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('approved', 'denied')),
    reason TEXT NOT NULL,
    decided_at TIMESTAMP NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO checkpoint_decisions_next (
    issue_uid,
    board_id,
    outcome,
    reason,
    decided_at,
    revision
)
SELECT issue.uid, decision.board_id, decision.outcome,
    decision.reason, decision.decided_at, decision.revision
FROM checkpoint_decisions AS decision
JOIN issues_next AS issue
    ON issue.board_id = decision.board_id
    AND issue.id = decision.issue_id;

CREATE TABLE attachments_next (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    id TEXT NOT NULL CHECK (
        length(id) = 30
        AND substr(id, 1, 4) = 'att_'
        AND substr(id, 5) NOT GLOB '*[^a-z2-7]*'
        AND substr(id, 30, 1) IN ('a', 'e', 'i', 'm', 'q', 'u', 'y', '4')
    ),
    origin_issue_uid BLOB,
    blob_digest TEXT NOT NULL,
    blob_size_bytes INTEGER NOT NULL,
    filename TEXT NOT NULL CHECK (
        length(CAST(filename AS BLOB)) BETWEEN 1 AND 255
    ),
    media_type TEXT NOT NULL CHECK (length(trim(media_type)) > 0),
    lifecycle TEXT NOT NULL CHECK (lifecycle IN ('active', 'removed')),
    created_actor TEXT NOT NULL CHECK (
        length(created_actor) > 0 AND created_actor = trim(created_actor)
    ),
    created_at TIMESTAMP NOT NULL,
    created_revision INTEGER NOT NULL CHECK (created_revision > 0),
    removed_actor TEXT,
    removed_at TIMESTAMP,
    removed_revision INTEGER,
    PRIMARY KEY (board_id, id),
    FOREIGN KEY (board_id, origin_issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE RESTRICT,
    FOREIGN KEY (blob_digest, blob_size_bytes)
        REFERENCES attachment_blobs(digest, size_bytes) ON DELETE RESTRICT,
    CHECK (
        (
            lifecycle = 'active'
            AND removed_actor IS NULL
            AND removed_at IS NULL
            AND removed_revision IS NULL
        )
        OR (
            lifecycle = 'removed'
            AND removed_actor IS NOT NULL
            AND length(removed_actor) > 0
            AND removed_actor = trim(removed_actor)
            AND removed_at IS NOT NULL
            AND removed_at >= created_at
            AND removed_revision IS NOT NULL
            AND removed_revision > created_revision
        )
    )
);

INSERT INTO attachments_next (
    board_id,
    id,
    origin_issue_uid,
    blob_digest,
    blob_size_bytes,
    filename,
    media_type,
    lifecycle,
    created_actor,
    created_at,
    created_revision,
    removed_actor,
    removed_at,
    removed_revision
)
SELECT attachment.board_id, attachment.id, issue.uid,
    attachment.blob_digest, attachment.blob_size_bytes,
    attachment.filename, attachment.media_type, attachment.lifecycle,
    attachment.created_actor, attachment.created_at,
    attachment.created_revision, attachment.removed_actor,
    attachment.removed_at, attachment.removed_revision
FROM attachments AS attachment
LEFT JOIN issues_next AS issue
    ON issue.board_id = attachment.board_id
    AND issue.id = attachment.origin_issue_id;

CREATE TABLE attachment_uploads_next (
    id TEXT PRIMARY KEY CHECK (
        length(id) > 0
        AND instr(id, ' ') = 0
        AND instr(id, char(9)) = 0
        AND instr(id, char(10)) = 0
        AND instr(id, char(13)) = 0
    ),
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    origin_issue_uid BLOB,
    filename TEXT NOT NULL CHECK (
        length(CAST(filename AS BLOB)) BETWEEN 1 AND 255
    ),
    expected_size_bytes INTEGER CHECK (
        expected_size_bytes BETWEEN 0 AND 104857600
    ),
    expected_digest TEXT CHECK (
        expected_digest IS NULL
        OR (
            length(expected_digest) = 71
            AND substr(expected_digest, 1, 7) = 'sha256:'
            AND substr(expected_digest, 8) NOT GLOB '*[^0-9a-f]*'
        )
    ),
    actor TEXT NOT NULL CHECK (
        length(actor) > 0 AND actor = trim(actor)
    ),
    state TEXT NOT NULL CHECK (
        state IN ('active', 'committed', 'aborted', 'expired')
    ),
    accepted_offset INTEGER NOT NULL CHECK (
        accepted_offset BETWEEN 0 AND 104857600
        AND (
            expected_size_bytes IS NULL
            OR accepted_offset <= expected_size_bytes
        )
    ),
    expires_at TIMESTAMP NOT NULL,
    attachment_id TEXT,
    admitted_max_bytes INTEGER NOT NULL DEFAULT 104857600 CHECK (
        admitted_max_bytes > 0
    ),
    FOREIGN KEY (board_id, origin_issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE RESTRICT,
    FOREIGN KEY (board_id, attachment_id)
        REFERENCES attachments_next(board_id, id) ON DELETE RESTRICT,
    CHECK (
        (state = 'committed' AND attachment_id IS NOT NULL)
        OR (state <> 'committed' AND attachment_id IS NULL)
    )
);

INSERT INTO attachment_uploads_next (
    id,
    board_id,
    origin_issue_uid,
    filename,
    expected_size_bytes,
    expected_digest,
    actor,
    state,
    accepted_offset,
    expires_at,
    attachment_id,
    admitted_max_bytes
)
SELECT upload.id, upload.board_id, issue.uid, upload.filename,
    upload.expected_size_bytes, upload.expected_digest, upload.actor,
    upload.state, upload.accepted_offset, upload.expires_at,
    upload.attachment_id, upload.admitted_max_bytes
FROM attachment_uploads AS upload
LEFT JOIN issues_next AS issue
    ON issue.board_id = upload.board_id
    AND issue.id = upload.origin_issue_id;

CREATE TABLE board_pins_next (
    board_id TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    position INTEGER NOT NULL CHECK (position > 0),
    PRIMARY KEY (board_id, issue_uid),
    UNIQUE (board_id, position),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues_next(board_id, uid) ON DELETE CASCADE
);

INSERT INTO board_pins_next (board_id, issue_uid, position)
SELECT pin.board_id, issue.uid, pin.position
FROM board_pins AS pin
JOIN issues_next AS issue
    ON issue.board_id = pin.board_id
    AND issue.id = pin.issue_id;

-- Every old row must have reached its replacement. This turns damaged legacy
-- references into a migration failure instead of allowing an inner join or an
-- optional origin join to discard association data.
CREATE TABLE issue_uid_migration_guard (
    valid INTEGER NOT NULL CHECK (valid = 1)
);

INSERT INTO issue_uid_migration_guard (valid)
SELECT
    (SELECT count(*) FROM issues) = (SELECT count(*) FROM issues_next)
    AND (SELECT count(*) FROM active_claims) = (SELECT count(*) FROM active_claims_next)
    AND (SELECT count(*) FROM issue_labels) = (SELECT count(*) FROM issue_labels_next)
    AND (SELECT count(*) FROM dependencies) = (SELECT count(*) FROM dependencies_next)
    AND (SELECT count(*) FROM containment) = (SELECT count(*) FROM containment_next)
    AND (SELECT count(*) FROM issue_external_keys) = (SELECT count(*) FROM issue_external_keys_next)
    AND (SELECT count(*) FROM issue_results) = (SELECT count(*) FROM issue_results_next)
    AND (SELECT count(*) FROM issue_log_entries) = (SELECT count(*) FROM issue_log_entries_next)
    AND (SELECT count(*) FROM issue_states) = (SELECT count(*) FROM issue_states_next)
    AND (SELECT count(*) FROM checkpoint_decisions) = (SELECT count(*) FROM checkpoint_decisions_next)
    AND (SELECT count(*) FROM attachments) = (SELECT count(*) FROM attachments_next)
    AND (SELECT count(*) FROM attachment_uploads) = (SELECT count(*) FROM attachment_uploads_next)
    AND (SELECT count(*) FROM board_pins) = (SELECT count(*) FROM board_pins_next)
    AND (
        SELECT count(*) FROM attachments WHERE origin_issue_id IS NOT NULL
    ) = (
        SELECT count(*) FROM attachments_next WHERE origin_issue_uid IS NOT NULL
    )
    AND (
        SELECT count(*) FROM attachment_uploads WHERE origin_issue_id IS NOT NULL
    ) = (
        SELECT count(*) FROM attachment_uploads_next WHERE origin_issue_uid IS NOT NULL
    );

DROP TABLE issue_uid_migration_guard;

-- Remove every old issue-owned table before replacing the issue table. The
-- new tables reference only other replacement tables during this phase.
DROP TABLE attachment_uploads;
DROP TABLE attachments;
DROP TABLE board_pins;
DROP TABLE issue_states;
DROP TABLE checkpoint_decisions;
DROP TABLE issue_log_entries;
DROP TABLE issue_results;
DROP TABLE issue_external_keys;
DROP TABLE containment;
DROP TABLE dependencies;
DROP TABLE issue_labels;
DROP TABLE active_claims;
DROP TABLE issues;

ALTER TABLE issues_next RENAME TO issues;
ALTER TABLE active_claims_next RENAME TO active_claims;
ALTER TABLE issue_labels_next RENAME TO issue_labels;
ALTER TABLE dependencies_next RENAME TO dependencies;
ALTER TABLE containment_next RENAME TO containment;
ALTER TABLE issue_external_keys_next RENAME TO issue_external_keys;
ALTER TABLE issue_results_next RENAME TO issue_results;
ALTER TABLE issue_log_entries_next RENAME TO issue_log_entries;
ALTER TABLE issue_states_next RENAME TO issue_states;
ALTER TABLE checkpoint_decisions_next RENAME TO checkpoint_decisions;
ALTER TABLE attachments_next RENAME TO attachments;
ALTER TABLE attachment_uploads_next RENAME TO attachment_uploads;
ALTER TABLE board_pins_next RENAME TO board_pins;

-- Restore the predecessor high-water after the replacement has its final name.
UPDATE sqlite_sequence
SET seq = max(seq, (
    SELECT high_water FROM issue_log_entries_sequence_guard
))
WHERE name = 'issue_log_entries';

INSERT INTO sqlite_sequence (name, seq)
SELECT 'issue_log_entries', high_water
FROM issue_log_entries_sequence_guard
WHERE high_water > 0
    AND NOT EXISTS (
        SELECT 1
        FROM sqlite_sequence
        WHERE name = 'issue_log_entries'
    );

DROP TABLE issue_log_entries_sequence_guard;

CREATE INDEX issues_by_lifecycle
    ON issues (board_id, lifecycle, priority, created_at, id);
CREATE INDEX issues_by_kind
    ON issues (board_id, kind, lifecycle, id);
CREATE INDEX issues_by_update
    ON issues (board_id, updated_at, id);

-- +goose StatementBegin
CREATE TRIGGER issues_reject_uid_update
BEFORE UPDATE OF uid ON issues
WHEN NEW.uid IS NOT OLD.uid
BEGIN
    SELECT RAISE(ABORT, 'issue UIDs are immutable');
END;
-- +goose StatementEnd

CREATE INDEX active_claims_by_actor ON active_claims (actor, issue_uid);
CREATE INDEX active_claims_by_start ON active_claims (started_at, issue_uid);
CREATE INDEX issue_labels_by_label
    ON issue_labels (board_id, label, issue_uid);
CREATE INDEX dependencies_by_prerequisite
    ON dependencies (prerequisite_uid, issue_uid);
CREATE INDEX containment_by_parent ON containment (parent_uid, child_uid);
CREATE INDEX issue_external_keys_by_issue
    ON issue_external_keys (issue_uid, external_key);
CREATE INDEX issue_log_entries_by_issue
    ON issue_log_entries (issue_uid, local_sequence);
CREATE INDEX issue_log_entries_by_board
    ON issue_log_entries (board_id, local_sequence);

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_validate_next_action_insert
BEFORE INSERT ON issue_log_entries
WHEN NEW.kind = 'post' AND NEW.next_action IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'Log posts cannot carry a next action');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_validate_next_action_update
BEFORE UPDATE OF kind, next_action ON issue_log_entries
WHEN NEW.kind = 'post' AND NEW.next_action IS NOT NULL
BEGIN
    SELECT RAISE(ABORT, 'Log posts cannot carry a next action');
END;
-- +goose StatementEnd

-- Snapshot linkage names one immutable State snapshot for the same issue UID.
-- +goose StatementBegin
CREATE TRIGGER issue_states_validate_snapshot_insert
BEFORE INSERT ON issue_states
WHEN NEW.snapshot_log_entry_id IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM issue_log_entries
        WHERE issue_uid = NEW.issue_uid
            AND id = NEW.snapshot_log_entry_id
            AND kind = 'state_snapshot'
    )
BEGIN
    SELECT RAISE(ABORT, 'State snapshot linkage requires a matching snapshot');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_states_validate_snapshot_update
BEFORE UPDATE OF snapshot_log_entry_id ON issue_states
WHEN NEW.snapshot_log_entry_id IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM issue_log_entries
        WHERE issue_uid = NEW.issue_uid
            AND id = NEW.snapshot_log_entry_id
            AND kind = 'state_snapshot'
    )
BEGIN
    SELECT RAISE(ABORT, 'State snapshot linkage requires a matching snapshot');
END;
-- +goose StatementEnd

CREATE INDEX attachments_by_origin
    ON attachments (board_id, origin_issue_uid, id);
CREATE INDEX attachments_by_blob
    ON attachments (blob_digest, board_id, id);
CREATE INDEX attachments_by_lifecycle
    ON attachments (board_id, lifecycle, id);

-- +goose StatementBegin
CREATE TRIGGER attachments_reject_metadata_update
BEFORE UPDATE ON attachments
WHEN NEW.board_id IS NOT OLD.board_id
    OR NEW.id IS NOT OLD.id
    OR NEW.origin_issue_uid IS NOT OLD.origin_issue_uid
    OR NEW.blob_digest IS NOT OLD.blob_digest
    OR NEW.blob_size_bytes IS NOT OLD.blob_size_bytes
    OR NEW.filename IS NOT OLD.filename
    OR NEW.media_type IS NOT OLD.media_type
    OR NEW.created_actor IS NOT OLD.created_actor
    OR NEW.created_at IS NOT OLD.created_at
    OR NEW.created_revision IS NOT OLD.created_revision
BEGIN
    SELECT RAISE(ABORT, 'attachment metadata is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attachments_reject_tombstone_update
BEFORE UPDATE ON attachments
WHEN OLD.lifecycle = 'removed'
    AND (
        NEW.lifecycle IS NOT OLD.lifecycle
        OR NEW.removed_actor IS NOT OLD.removed_actor
        OR NEW.removed_at IS NOT OLD.removed_at
        OR NEW.removed_revision IS NOT OLD.removed_revision
    )
BEGIN
    SELECT RAISE(ABORT, 'attachment tombstones are immutable');
END;
-- +goose StatementEnd

-- A committed upload and its attachment must retain the same private origin.
-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_validate_committed_insert
BEFORE INSERT ON attachment_uploads
WHEN NEW.state = 'committed'
    AND NOT EXISTS (
        SELECT 1
        FROM attachments
        WHERE board_id = NEW.board_id
            AND id = NEW.attachment_id
            AND origin_issue_uid IS NEW.origin_issue_uid
            AND filename = NEW.filename
            AND lifecycle = 'active'
            AND created_actor = NEW.actor
            AND blob_size_bytes = NEW.accepted_offset
            AND (
                NEW.expected_size_bytes IS NULL
                OR blob_size_bytes = NEW.expected_size_bytes
            )
            AND (
                NEW.expected_digest IS NULL
                OR blob_digest = NEW.expected_digest
            )
    )
BEGIN
    SELECT RAISE(ABORT, 'committed upload must match an active attachment');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_validate_committed_update
BEFORE UPDATE ON attachment_uploads
WHEN NEW.state = 'committed'
    AND NOT EXISTS (
        SELECT 1
        FROM attachments
        WHERE board_id = NEW.board_id
            AND id = NEW.attachment_id
            AND origin_issue_uid IS NEW.origin_issue_uid
            AND filename = NEW.filename
            AND lifecycle = 'active'
            AND created_actor = NEW.actor
            AND blob_size_bytes = NEW.accepted_offset
            AND (
                NEW.expected_size_bytes IS NULL
                OR blob_size_bytes = NEW.expected_size_bytes
            )
            AND (
                NEW.expected_digest IS NULL
                OR blob_digest = NEW.expected_digest
            )
    )
BEGIN
    SELECT RAISE(ABORT, 'committed upload must match an active attachment');
END;
-- +goose StatementEnd

CREATE INDEX attachment_uploads_by_expiry
    ON attachment_uploads (state, expires_at, id);
CREATE INDEX attachment_uploads_by_board
    ON attachment_uploads (board_id, id);

-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_reject_identity_update
BEFORE UPDATE ON attachment_uploads
WHEN NEW.id IS NOT OLD.id
    OR NEW.board_id IS NOT OLD.board_id
    OR NEW.origin_issue_uid IS NOT OLD.origin_issue_uid
    OR NEW.filename IS NOT OLD.filename
    OR NEW.expected_size_bytes IS NOT OLD.expected_size_bytes
    OR NEW.expected_digest IS NOT OLD.expected_digest
    OR NEW.actor IS NOT OLD.actor
BEGIN
    SELECT RAISE(ABORT, 'attachment upload identity is immutable');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_reject_offset_rewind
BEFORE UPDATE ON attachment_uploads
WHEN NEW.accepted_offset < OLD.accepted_offset
BEGIN
    SELECT RAISE(ABORT, 'attachment upload offset cannot move backward');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_reject_terminal_update
BEFORE UPDATE ON attachment_uploads
WHEN OLD.state <> 'active'
BEGIN
    SELECT RAISE(ABORT, 'terminal attachment upload receipts are immutable');
END;
-- +goose StatementEnd

-- Full-text search is a derived projection of canonical issue records.
-- One document per field or Log entry preserves result provenance and keeps
-- large Log histories from accumulating relevance across several records.
CREATE TABLE issue_search_documents (
    rowid INTEGER PRIMARY KEY,
    board_id TEXT NOT NULL,
    issue_uid BLOB NOT NULL,
    field TEXT NOT NULL CHECK (
        field IN ('title', 'summary', 'details', 'state', 'result', 'log')
    ),
    record_id TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL CHECK (length(trim(body)) > 0),
    FOREIGN KEY (board_id, issue_uid)
        REFERENCES issues(board_id, uid) ON DELETE CASCADE,
    UNIQUE (issue_uid, field, record_id),
    CHECK (
        (field = 'log' AND record_id <> '')
        OR (field <> 'log' AND record_id = '')
    )
);

CREATE INDEX issue_search_documents_by_board_issue
    ON issue_search_documents (board_id, issue_uid, field, record_id);

-- The content table keeps board and record ownership available to repository
-- queries while FTS5 owns only the token index for each document body.
CREATE VIRTUAL TABLE issue_search_fts USING fts5(
    body,
    content = 'issue_search_documents',
    content_rowid = 'rowid',
    tokenize = 'unicode61'
);

-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_insert
AFTER INSERT ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(rowid, body) VALUES (NEW.rowid, NEW.body);
END;
-- +goose StatementEnd

-- External-content FTS5 tables require a delete command that includes the
-- indexed value which previously occupied the row.
-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_delete
AFTER DELETE ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(issue_search_fts, rowid, body)
    VALUES ('delete', OLD.rowid, OLD.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_search_documents_fts_update
AFTER UPDATE OF body ON issue_search_documents
BEGIN
    INSERT INTO issue_search_fts(issue_search_fts, rowid, body)
    VALUES ('delete', OLD.rowid, OLD.body);
    INSERT INTO issue_search_fts(rowid, body) VALUES (NEW.rowid, NEW.body);
END;
-- +goose StatementEnd

-- Issue scalar records share one canonical row. Replacing optional fields
-- removes absent documents before upserting the retained values.
-- +goose StatementBegin
CREATE TRIGGER issues_search_insert
AFTER INSERT ON issues
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    VALUES (NEW.board_id, NEW.uid, 'title', NEW.title);
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    SELECT NEW.board_id, NEW.uid, 'summary', NEW.summary
    WHERE NEW.summary IS NOT NULL;
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    SELECT NEW.board_id, NEW.uid, 'details', NEW.details
    WHERE NEW.details IS NOT NULL;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issues_search_update
AFTER UPDATE OF title, summary, details ON issues
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    VALUES (NEW.board_id, NEW.uid, 'title', NEW.title)
    ON CONFLICT(issue_uid, field, record_id)
    DO UPDATE SET body = excluded.body;

    DELETE FROM issue_search_documents
    WHERE issue_uid = NEW.uid AND field = 'summary' AND NEW.summary IS NULL;
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    SELECT NEW.board_id, NEW.uid, 'summary', NEW.summary
    WHERE NEW.summary IS NOT NULL
    ON CONFLICT(issue_uid, field, record_id)
    DO UPDATE SET body = excluded.body;

    DELETE FROM issue_search_documents
    WHERE issue_uid = NEW.uid AND field = 'details' AND NEW.details IS NULL;
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    SELECT NEW.board_id, NEW.uid, 'details', NEW.details
    WHERE NEW.details IS NOT NULL
    ON CONFLICT(issue_uid, field, record_id)
    DO UPDATE SET body = excluded.body;
END;
-- +goose StatementEnd

-- Delete documents before the issue row so the document trigger can remove
-- the corresponding external-content FTS5 entries.
-- +goose StatementBegin
CREATE TRIGGER issues_search_delete
BEFORE DELETE ON issues
BEGIN
    DELETE FROM issue_search_documents WHERE issue_uid = OLD.uid;
END;
-- +goose StatementEnd

-- State documents combine the mutable body and next action because both form
-- the current recovery position exposed by the State record.
-- +goose StatementBegin
CREATE TRIGGER issue_states_search_insert
AFTER INSERT ON issue_states
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    VALUES (
        NEW.board_id,
        NEW.issue_uid,
        'state',
        NEW.body || CASE
            WHEN NEW.next_action IS NULL THEN ''
            ELSE char(10) || char(10) || NEW.next_action
        END
    );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_states_search_update
AFTER UPDATE OF body, next_action ON issue_states
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body || CASE
        WHEN NEW.next_action IS NULL THEN ''
        ELSE char(10) || char(10) || NEW.next_action
    END
    WHERE issue_uid = NEW.issue_uid AND field = 'state';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_states_search_delete
AFTER DELETE ON issue_states
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_uid = OLD.issue_uid AND field = 'state';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_insert
AFTER INSERT ON issue_results
BEGIN
    INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
    VALUES (NEW.board_id, NEW.issue_uid, 'result', NEW.body);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_update
AFTER UPDATE OF body ON issue_results
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body
    WHERE issue_uid = NEW.issue_uid AND field = 'result';
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_results_search_delete
AFTER DELETE ON issue_results
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_uid = OLD.issue_uid AND field = 'result';
END;
-- +goose StatementEnd

-- A Log document retains the stable Log ID used for direct follow-up reads.
-- State snapshots include their preserved next action in the searchable body.
-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_insert
AFTER INSERT ON issue_log_entries
BEGIN
    INSERT INTO issue_search_documents(
        board_id,
        issue_uid,
        field,
        record_id,
        body
    ) VALUES (
        NEW.board_id,
        NEW.issue_uid,
        'log',
        NEW.id,
        NEW.body || CASE
            WHEN NEW.next_action IS NULL THEN ''
            ELSE char(10) || char(10) || NEW.next_action
        END
    );
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_update
AFTER UPDATE OF body, next_action ON issue_log_entries
BEGIN
    UPDATE issue_search_documents
    SET body = NEW.body || CASE
        WHEN NEW.next_action IS NULL THEN ''
        ELSE char(10) || char(10) || NEW.next_action
    END
    WHERE issue_uid = NEW.issue_uid
        AND field = 'log'
        AND record_id = NEW.id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issue_log_entries_search_delete
AFTER DELETE ON issue_log_entries
BEGIN
    DELETE FROM issue_search_documents
    WHERE issue_uid = OLD.issue_uid
        AND field = 'log'
        AND record_id = OLD.id;
END;
-- +goose StatementEnd

-- Backfill every current canonical record. The rebuild makes the final FTS5
-- index depend only on the completed document projection.
INSERT INTO issue_search_documents(board_id, issue_uid, field, body)
SELECT board_id, uid, 'title', title FROM issues
UNION ALL
SELECT board_id, uid, 'summary', summary FROM issues WHERE summary IS NOT NULL
UNION ALL
SELECT board_id, uid, 'details', details FROM issues WHERE details IS NOT NULL
UNION ALL
SELECT
    board_id,
    issue_uid,
    'state',
    body || CASE
        WHEN next_action IS NULL THEN ''
        ELSE char(10) || char(10) || next_action
    END
FROM issue_states
UNION ALL
SELECT board_id, issue_uid, 'result', body FROM issue_results;

INSERT INTO issue_search_documents(
    board_id,
    issue_uid,
    field,
    record_id,
    body
)
SELECT
    board_id,
    issue_uid,
    'log',
    id,
    body || CASE
        WHEN next_action IS NULL THEN ''
        ELSE char(10) || char(10) || next_action
    END
FROM issue_log_entries;
