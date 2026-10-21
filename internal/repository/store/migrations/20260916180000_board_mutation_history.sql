-- +goose Up

-- Payload parts are bounded so migration, verification, and future transport
-- can stream a mutation without materializing a complete board or mutation.
-- The deferred foreign key permits parts and their terminal commit to be
-- inserted in natural stream order within one transaction.
CREATE TABLE board_mutation_parts (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    writer_uid BLOB NOT NULL CHECK (
        typeof(writer_uid) = 'blob'
        AND length(writer_uid) = 16
        AND writer_uid <> zeroblob(16)
    ),
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    part_index INTEGER NOT NULL CHECK (part_index >= 0),
    bytes BLOB NOT NULL CHECK (
        typeof(bytes) = 'blob'
        AND length(bytes) BETWEEN 1 AND 65536
    ),
    PRIMARY KEY (board_id, writer_uid, sequence, part_index),
    FOREIGN KEY (board_id, writer_uid, sequence)
        REFERENCES board_mutation_commits (board_id, writer_uid, sequence)
        ON DELETE RESTRICT DEFERRABLE INITIALLY DEFERRED
);

-- One terminal marker commits the preceding parts. The retained marker bytes
-- are the authority for the observed writer frontier and the input to
-- marker_hash; the other columns support bounded verification without
-- interpreting an untrusted marker first.
CREATE TABLE board_mutation_commits (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    writer_uid BLOB NOT NULL CHECK (
        typeof(writer_uid) = 'blob'
        AND length(writer_uid) = 16
        AND writer_uid <> zeroblob(16)
    ),
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    previous_hash BLOB NOT NULL CHECK (
        typeof(previous_hash) = 'blob' AND length(previous_hash) = 32
    ),
    marker_hash BLOB NOT NULL CHECK (
        typeof(marker_hash) = 'blob' AND length(marker_hash) = 32
    ),
    marker_bytes BLOB NOT NULL CHECK (
        typeof(marker_bytes) = 'blob'
        AND length(marker_bytes) BETWEEN 1 AND 65536
    ),
    part_count INTEGER NOT NULL CHECK (part_count > 0),
    event_count INTEGER NOT NULL CHECK (event_count > 0),
    payload_hash BLOB NOT NULL CHECK (
        typeof(payload_hash) = 'blob' AND length(payload_hash) = 32
    ),
    PRIMARY KEY (board_id, writer_uid, sequence),
    UNIQUE (board_id, writer_uid, sequence, marker_hash)
);

-- A tip points only to a complete committed marker. Sequence and digest are
-- repeated so a reader can locate the final mutation without scanning parts.
CREATE TABLE board_mutation_tips (
    board_id TEXT NOT NULL REFERENCES boards(id) ON DELETE RESTRICT,
    writer_uid BLOB NOT NULL,
    sequence INTEGER NOT NULL,
    marker_hash BLOB NOT NULL,
    PRIMARY KEY (board_id, writer_uid),
    FOREIGN KEY (board_id, writer_uid, sequence, marker_hash)
        REFERENCES board_mutation_commits (
            board_id, writer_uid, sequence, marker_hash
        ) ON DELETE RESTRICT
);

-- History retains attachment blobs independently of the current attachment
-- projection. Blob collection must include these references before current
-- metadata can stop retaining a historical blob.
CREATE TABLE board_mutation_blob_refs (
    board_id TEXT NOT NULL,
    writer_uid BLOB NOT NULL,
    sequence INTEGER NOT NULL,
    digest TEXT NOT NULL,
    size_bytes INTEGER NOT NULL,
    PRIMARY KEY (board_id, writer_uid, sequence, digest),
    FOREIGN KEY (board_id, writer_uid, sequence)
        REFERENCES board_mutation_commits (board_id, writer_uid, sequence)
        ON DELETE RESTRICT,
    FOREIGN KEY (digest, size_bytes)
        REFERENCES attachment_blobs (digest, size_bytes) ON DELETE RESTRICT
);

CREATE INDEX board_mutation_blob_refs_by_digest
    ON board_mutation_blob_refs (digest);

-- +goose Down

DROP INDEX board_mutation_blob_refs_by_digest;
DROP TABLE board_mutation_blob_refs;
DROP TABLE board_mutation_tips;
DROP TABLE board_mutation_parts;
DROP TABLE board_mutation_commits;
