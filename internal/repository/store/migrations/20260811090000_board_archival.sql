-- +goose Up

-- These columns hold current, reversible archive state rather than a history.
-- Active boards have all three values null; archived boards require time and
-- actor while reason remains optional.
ALTER TABLE boards ADD COLUMN archived_at TIMESTAMP;
ALTER TABLE boards ADD COLUMN archived_by TEXT;
ALTER TABLE boards ADD COLUMN archive_reason TEXT;

-- Enforce the whole-value invariant for imports and other direct writers too.
-- +goose StatementBegin
CREATE TRIGGER boards_archive_metadata_insert
BEFORE INSERT ON boards
WHEN NOT (
    (NEW.archived_at IS NULL AND NEW.archived_by IS NULL AND NEW.archive_reason IS NULL)
    OR (
        NEW.archived_at IS NOT NULL
        AND NEW.archived_by IS NOT NULL
        AND length(trim(NEW.archived_by)) > 0
        AND (NEW.archive_reason IS NULL OR length(trim(NEW.archive_reason)) > 0)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'invalid board archive metadata');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER boards_archive_metadata_update
BEFORE UPDATE OF archived_at, archived_by, archive_reason ON boards
WHEN NOT (
    (NEW.archived_at IS NULL AND NEW.archived_by IS NULL AND NEW.archive_reason IS NULL)
    OR (
        NEW.archived_at IS NOT NULL
        AND NEW.archived_by IS NOT NULL
        AND length(trim(NEW.archived_by)) > 0
        AND (NEW.archive_reason IS NULL OR length(trim(NEW.archive_reason)) > 0)
    )
)
BEGIN
    SELECT RAISE(ABORT, 'invalid board archive metadata');
END;
-- +goose StatementEnd

-- Active catalog reads lead with archived_at and retain their display order.
CREATE INDEX boards_by_archive ON boards (archived_at, project_id, name, id);

-- +goose Down

DROP INDEX boards_by_archive;
DROP TRIGGER boards_archive_metadata_update;
DROP TRIGGER boards_archive_metadata_insert;
ALTER TABLE boards DROP COLUMN archive_reason;
ALTER TABLE boards DROP COLUMN archived_by;
ALTER TABLE boards DROP COLUMN archived_at;
