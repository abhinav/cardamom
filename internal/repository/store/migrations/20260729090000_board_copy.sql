-- +goose Up

-- Store lineage identifies the persistence history shared by filesystem
-- clones. The random value is created once for every existing and new store.
CREATE TABLE store_lineage (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    id TEXT NOT NULL UNIQUE CHECK (
        length(id) = 38
        AND substr(id, 1, 6) = 'store_'
        AND substr(id, 7) NOT GLOB '*[^0-9a-f]*'
    )
);

INSERT INTO store_lineage (singleton, id)
VALUES (1, 'store_' || lower(hex(randomblob(16))));

-- A receipt records one successfully published board snapshot and the options
-- that selected its destination. Multiple snapshots from one source board are
-- representable; the v1 command's changed-snapshot policy remains in code.
CREATE TABLE board_copy_receipts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_lineage_id TEXT NOT NULL,
    source_board_id TEXT NOT NULL,
    snapshot_version INTEGER NOT NULL CHECK (snapshot_version > 0),
    snapshot_digest TEXT NOT NULL CHECK (
        length(snapshot_digest) = 71
        AND substr(snapshot_digest, 1, 7) = 'sha256:'
        AND substr(snapshot_digest, 8) NOT GLOB '*[^0-9a-f]*'
    ),
    source_revision INTEGER NOT NULL CHECK (source_revision >= 0),
    destination_project_id TEXT NOT NULL
        REFERENCES projects(id) ON DELETE RESTRICT,
    destination_name TEXT NOT NULL CHECK (length(trim(destination_name)) > 0),
    destination_board_id TEXT NOT NULL
        REFERENCES boards(id) ON DELETE RESTRICT,
    destination_revision INTEGER NOT NULL CHECK (destination_revision > 0),
    created_at TIMESTAMP NOT NULL,
    UNIQUE (
        source_lineage_id,
        source_board_id,
        snapshot_version,
        snapshot_digest,
        destination_project_id,
        destination_name
    )
);

CREATE INDEX board_copy_receipts_by_source
    ON board_copy_receipts (
        source_lineage_id,
        source_board_id,
        snapshot_version,
        id
    );

-- Mapping rows preserve the identity decision for every copied object,
-- including objects whose source and destination identities are equal.
CREATE TABLE board_copy_issue_mappings (
    receipt_id INTEGER NOT NULL
        REFERENCES board_copy_receipts(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    PRIMARY KEY (receipt_id, source_id),
    UNIQUE (receipt_id, destination_id)
);

CREATE TABLE board_copy_log_mappings (
    receipt_id INTEGER NOT NULL
        REFERENCES board_copy_receipts(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    PRIMARY KEY (receipt_id, source_id),
    UNIQUE (receipt_id, destination_id)
);

CREATE TABLE board_copy_attachment_mappings (
    receipt_id INTEGER NOT NULL
        REFERENCES board_copy_receipts(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    PRIMARY KEY (receipt_id, source_id),
    UNIQUE (receipt_id, destination_id)
);
