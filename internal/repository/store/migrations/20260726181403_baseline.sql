-- +goose Up

-- Canonical store publication state.
--
-- The singleton store state publishes the canonical revision and owns the
-- store-wide issue-number allocator. Repository changes publish projection
-- rows and the next canonical revision in one SQLite transaction.
CREATE TABLE store_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    current_revision INTEGER NOT NULL CHECK (current_revision >= 0),
    next_issue_number INTEGER NOT NULL CHECK (next_issue_number > 0)
);

INSERT INTO store_state (singleton, current_revision, next_issue_number)
VALUES (1, 0, 1);

-- Project and board scope.
--
-- Projects partition logical repository or product namespaces within one
-- physical store. Boards are explicit coordination scopes within a project;
-- issue state below cannot outlive either enclosing scope.
--
-- Nullable configuration columns inherit from the preceding configuration
-- layer. Limits govern later write admission and therefore do not constrain
-- retained issue summaries or attachment metadata.
CREATE TABLE projects (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    created_at TIMESTAMP NOT NULL,
    issue_id_prefix TEXT CHECK (
        issue_id_prefix IS NULL
        OR (
            length(issue_id_prefix) BETWEEN 1 AND 16
            AND issue_id_prefix = lower(issue_id_prefix)
            AND issue_id_prefix NOT GLOB '*[^a-z0-9-]*'
            AND substr(issue_id_prefix, -1) = '-'
        )
    ),
    issue_id_strategy TEXT CHECK (
        issue_id_strategy IS NULL
        OR issue_id_strategy IN ('random', 'sequential')
    ),
    issue_summary_max_bytes INTEGER CHECK (
        issue_summary_max_bytes IS NULL
        OR issue_summary_max_bytes > 0
    ),
    attachment_max_bytes INTEGER CHECK (
        attachment_max_bytes IS NULL
        OR attachment_max_bytes > 0
    )
);

CREATE INDEX projects_by_name ON projects (name, id);

CREATE TABLE boards (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
    name TEXT NOT NULL CHECK (length(trim(name)) > 0),
    description TEXT CHECK (
        description IS NULL OR length(trim(description)) > 0
    ),
    created_at TIMESTAMP NOT NULL,
    issue_id_prefix TEXT CHECK (
        issue_id_prefix IS NULL
        OR (
            length(issue_id_prefix) BETWEEN 1 AND 16
            AND issue_id_prefix = lower(issue_id_prefix)
            AND issue_id_prefix NOT GLOB '*[^a-z0-9-]*'
            AND substr(issue_id_prefix, -1) = '-'
        )
    ),
    issue_id_strategy TEXT CHECK (
        issue_id_strategy IS NULL
        OR issue_id_strategy IN ('random', 'sequential')
    ),
    issue_summary_max_bytes INTEGER CHECK (
        issue_summary_max_bytes IS NULL
        OR issue_summary_max_bytes > 0
    ),
    attachment_max_bytes INTEGER CHECK (
        attachment_max_bytes IS NULL
        OR attachment_max_bytes > 0
    ),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0)
);

CREATE INDEX boards_by_project_name ON boards (project_id, name, id);
CREATE INDEX boards_by_name ON boards (name, id);

-- Issue lifecycle, custody, and graph metadata.
--
-- Issue-owned tables repeat board_id and reference issues through the composite
-- key so claims, labels, relationships, keys, and records cannot cross board
-- scope. Claim custody is separate from lifecycle and limited to one active
-- actor per issue; store verification rejects claims on closed, cancelled, or
-- non-executable issues.
--
-- Dependencies control readiness, while containment controls inherited context
-- and permits only one parent per child. SQL rejects self-edges; repository
-- operations preserve the broader acyclic dependency and containment graphs.
CREATE TABLE issues (
    id TEXT PRIMARY KEY CHECK (
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
    UNIQUE (board_id, id),
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

CREATE INDEX issues_by_lifecycle
    ON issues (board_id, lifecycle, priority, created_at, id);
CREATE INDEX issues_by_kind
    ON issues (board_id, kind, lifecycle, id);
CREATE INDEX issues_by_update
    ON issues (board_id, updated_at, id);

CREATE TABLE active_claims (
    issue_id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL,
    actor TEXT NOT NULL CHECK (length(trim(actor)) > 0),
    started_at TIMESTAMP NOT NULL,
    started_revision INTEGER NOT NULL CHECK (started_revision > 0),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE INDEX active_claims_by_actor ON active_claims (actor, issue_id);
CREATE INDEX active_claims_by_start ON active_claims (started_at, issue_id);

CREATE TABLE issue_labels (
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    label TEXT NOT NULL CHECK (length(trim(label)) > 0),
    PRIMARY KEY (issue_id, label),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE INDEX issue_labels_by_label
    ON issue_labels (board_id, label, issue_id);

CREATE TABLE dependencies (
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    prerequisite_id TEXT NOT NULL,
    PRIMARY KEY (issue_id, prerequisite_id),
    CHECK (issue_id <> prerequisite_id),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE,
    FOREIGN KEY (board_id, prerequisite_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE INDEX dependencies_by_prerequisite
    ON dependencies (prerequisite_id, issue_id);

CREATE TABLE containment (
    board_id TEXT NOT NULL,
    child_id TEXT PRIMARY KEY,
    parent_id TEXT NOT NULL,
    CHECK (child_id <> parent_id),
    FOREIGN KEY (board_id, child_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE,
    FOREIGN KEY (board_id, parent_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE INDEX containment_by_parent ON containment (parent_id, child_id);

CREATE TABLE issue_external_keys (
    board_id TEXT NOT NULL,
    external_key TEXT NOT NULL,
    issue_id TEXT NOT NULL,
    PRIMARY KEY (board_id, external_key),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE INDEX issue_external_keys_by_issue
    ON issue_external_keys (issue_id, external_key);

-- Issue records.
--
-- Summary and details live on the issue as stable context. Results retain one
-- current durable outcome. State retains the replaceable recovery record, and
-- log entries retain the immutable chronological record.
-- Log IDs are durable public handles. The private local sequence preserves
-- repository-local ordering without crossing the repository boundary. Posts
-- require complete authorship and time; State snapshots may omit provenance.
CREATE TABLE issue_results (
    issue_id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL,
    body TEXT NOT NULL CHECK (length(trim(body)) > 0),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

CREATE TABLE issue_log_entries (
    local_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE CHECK (
        length(id) = 36
        AND substr(id, 1, 4) IN ('cmt_', 'log_')
        AND substr(id, 5) NOT GLOB '*[^0-9a-f]*'
    ),
    board_id TEXT NOT NULL,
    issue_id TEXT NOT NULL,
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
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE,
    UNIQUE (issue_id, id),
    CHECK (
        kind <> 'post'
        OR (
            author IS NOT NULL
            AND committer IS NOT NULL
            AND created_at IS NOT NULL
        )
    )
);

CREATE INDEX issue_log_entries_by_issue
    ON issue_log_entries (issue_id, local_sequence);
CREATE INDEX issue_log_entries_by_board
    ON issue_log_entries (board_id, local_sequence);

-- Log posts are standalone chronology and cannot represent a State transition.
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

-- State is the optional mutable recovery record.
--
-- Attribution and update time are either both known or both absent. Snapshot
-- linkage records the immutable State snapshot containing the same recovery
-- content; the triggers require that link to name a snapshot for the same
-- issue.
CREATE TABLE issue_states (
    issue_id TEXT PRIMARY KEY,
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
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE,
    FOREIGN KEY (issue_id, snapshot_log_entry_id)
        REFERENCES issue_log_entries(issue_id, id) ON DELETE RESTRICT,
    CHECK (
        (author IS NULL AND updated_at IS NULL)
        OR (author IS NOT NULL AND updated_at IS NOT NULL)
    )
);

-- +goose StatementBegin
CREATE TRIGGER issue_states_validate_snapshot_insert
BEFORE INSERT ON issue_states
WHEN NEW.snapshot_log_entry_id IS NOT NULL
    AND NOT EXISTS (
        SELECT 1
        FROM issue_log_entries
        WHERE issue_id = NEW.issue_id
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
        WHERE issue_id = NEW.issue_id
            AND id = NEW.snapshot_log_entry_id
            AND kind = 'state_snapshot'
    )
BEGIN
    SELECT RAISE(ABORT, 'State snapshot linkage requires a matching snapshot');
END;
-- +goose StatementEnd

-- Checkpoint decisions are immutable issue projections.
--
-- The revision identifies the repository change that resolved the checkpoint.
-- Actor attribution belongs to that change's external record rather than this
-- current-state projection.
CREATE TABLE checkpoint_decisions (
    issue_id TEXT PRIMARY KEY,
    board_id TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('approved', 'denied')),
    reason TEXT NOT NULL,
    decided_at TIMESTAMP NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    FOREIGN KEY (board_id, issue_id)
        REFERENCES issues(board_id, id) ON DELETE CASCADE
);

-- Content-addressed blob descriptors retained by attachment metadata.
--
-- Blob identity is store-wide so equal content has one descriptor across every
-- board. Attachment availability is observed from local bytes and is not part
-- of this logical schema. The size limit is recorded directly in migration
-- history so later product constants cannot change this historical contract.
CREATE TABLE attachment_blobs (
    digest TEXT PRIMARY KEY CHECK (
        length(digest) = 71
        AND substr(digest, 1, 7) = 'sha256:'
        AND substr(digest, 8) NOT GLOB '*[^0-9a-f]*'
    ),
    size_bytes INTEGER NOT NULL CHECK (
        size_bytes BETWEEN 0 AND 104857600
    ),
    UNIQUE (digest, size_bytes)
);

-- +goose StatementBegin
CREATE TRIGGER attachment_blobs_reject_update
BEFORE UPDATE ON attachment_blobs
BEGIN
    SELECT RAISE(ABORT, 'attachment blob descriptors are immutable');
END;
-- +goose StatementEnd

-- Board-scoped attachment metadata and monotonic tombstones.
--
-- The optional origin issue records where the attachment entered its board;
-- the composite foreign key prevents a cross-board association. Blob metadata,
-- presentation metadata, and creation attribution are immutable. Removal keeps
-- complete attribution and may advance only once from active to removed.
CREATE TABLE attachments (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    id TEXT NOT NULL CHECK (
        length(id) = 30
        AND substr(id, 1, 4) = 'att_'
        AND substr(id, 5) NOT GLOB '*[^a-z2-7]*'
        AND substr(id, 30, 1) IN ('a', 'e', 'i', 'm', 'q', 'u', 'y', '4')
    ),
    origin_issue_id TEXT,
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
    FOREIGN KEY (board_id, origin_issue_id)
        REFERENCES issues(board_id, id) ON DELETE RESTRICT,
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

CREATE INDEX attachments_by_origin
    ON attachments (board_id, origin_issue_id, id);
CREATE INDEX attachments_by_blob
    ON attachments (blob_digest, board_id, id);
CREATE INDEX attachments_by_lifecycle
    ON attachments (board_id, lifecycle, id);

-- +goose StatementBegin
CREATE TRIGGER attachments_reject_metadata_update
BEFORE UPDATE ON attachments
WHEN NEW.board_id IS NOT OLD.board_id
    OR NEW.id IS NOT OLD.id
    OR NEW.origin_issue_id IS NOT OLD.origin_issue_id
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

-- Durable resumable upload progress and terminal receipts.
--
-- Upload IDs are store-wide because recovery selects a session by ID alone.
-- Active sessions retain their accepted sequential offset and admitted maximum.
-- Only committed receipts name an attachment, and that attachment must belong
-- to the session's board. Terminal rows are immutable until receipt collection
-- deletes them.
CREATE TABLE attachment_uploads (
    id TEXT PRIMARY KEY CHECK (
        length(id) > 0
        AND instr(id, ' ') = 0
        AND instr(id, char(9)) = 0
        AND instr(id, char(10)) = 0
        AND instr(id, char(13)) = 0
    ),
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    origin_issue_id TEXT,
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
    FOREIGN KEY (board_id, origin_issue_id)
        REFERENCES issues(board_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (board_id, attachment_id)
        REFERENCES attachments(board_id, id) ON DELETE RESTRICT,
    CHECK (
        (state = 'committed' AND attachment_id IS NOT NULL)
        OR (state <> 'committed' AND attachment_id IS NULL)
    )
);

-- +goose StatementBegin
CREATE TRIGGER attachment_uploads_validate_committed_insert
BEFORE INSERT ON attachment_uploads
WHEN NEW.state = 'committed'
    AND NOT EXISTS (
        SELECT 1
        FROM attachments
        WHERE board_id = NEW.board_id
            AND id = NEW.attachment_id
            AND origin_issue_id IS NEW.origin_issue_id
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
            AND origin_issue_id IS NEW.origin_issue_id
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
    OR NEW.origin_issue_id IS NOT OLD.origin_issue_id
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

-- Ephemeral actor coordination.
--
-- Mail deliveries and topic subscriptions use store-wide actor namespaces,
-- independent of project and board scope. Expiration bounds their lifetime;
-- repository operations decide which active deliveries match and when consumed
-- messages become read.
-- Mail IDs are durable public handles, while local sequences preserve polling
-- order. A listener and pattern together identify one subscription.
CREATE TABLE mailbox (
    local_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE CHECK (
        length(id) = 37
        AND substr(id, 1, 5) = 'mail_'
        AND substr(id, 6) NOT GLOB '*[^0-9a-f]*'
    ),
    sender TEXT NOT NULL CHECK (length(trim(sender)) > 0),
    recipient TEXT NOT NULL CHECK (length(trim(recipient)) > 0),
    body TEXT NOT NULL CHECK (length(body) > 0),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    read_at TIMESTAMP,
    source_topic TEXT CHECK (
        source_topic IS NULL OR length(source_topic) > 0
    ),
    CHECK (expires_at > created_at)
);

CREATE INDEX mailbox_by_recipient
    ON mailbox (recipient, read_at, local_sequence);
CREATE INDEX mailbox_by_expiry ON mailbox (expires_at, local_sequence);

CREATE TABLE subscriptions (
    listener TEXT NOT NULL CHECK (length(trim(listener)) > 0),
    pattern TEXT NOT NULL CHECK (length(trim(pattern)) > 0),
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    PRIMARY KEY (listener, pattern),
    CHECK (expires_at > created_at)
);

CREATE INDEX subscriptions_by_listener ON subscriptions (listener, pattern);
CREATE INDEX subscriptions_by_expiry
    ON subscriptions (expires_at, listener, pattern);

-- Expiring resource custody.
--
-- Leases coordinate store-wide external resources independently of issue claim
-- custody. A worker that needs both kinds of ownership must hold both records.
CREATE TABLE leases (
    name TEXT PRIMARY KEY CHECK (length(trim(name)) > 0),
    owner TEXT NOT NULL CHECK (length(trim(owner)) > 0),
    acquired_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    CHECK (expires_at > acquired_at)
);

CREATE INDEX leases_by_expiry ON leases (expires_at, name);
