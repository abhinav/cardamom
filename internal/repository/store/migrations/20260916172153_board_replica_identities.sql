-- +goose Up

-- Each boards row materializes one replica of the logical board identified by
-- boards.id. The companion writer UID distinguishes that replica's future
-- mutation chain. A supported clone will preserve boards.id while assigning a
-- new writer UID; an independent copy assigns both identities anew.
CREATE TABLE board_replica_identities (
    board_id TEXT PRIMARY KEY
        REFERENCES boards(id) ON DELETE RESTRICT,
    writer_uid BLOB NOT NULL UNIQUE CHECK (
        typeof(writer_uid) = 'blob'
        AND length(writer_uid) = 16
        AND writer_uid <> zeroblob(16)
    )
);

-- Existing boards become distinct replicas. randomblob supplies one private
-- identity per row inside this migration transaction. A uniqueness collision
-- aborts the complete migration and leaves the preceding schema active.
INSERT INTO board_replica_identities (board_id, writer_uid)
SELECT id, randomblob(16)
FROM boards;
