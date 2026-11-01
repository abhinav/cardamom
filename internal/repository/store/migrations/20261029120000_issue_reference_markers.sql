-- +goose Up

-- Retired public IDs remain usable references and can never be reassigned.
-- A marker is not an executable issue and therefore has no board owner.
CREATE TABLE issue_reference_markers (
    id TEXT PRIMARY KEY NOT NULL
);

-- Candidates deliberately have no issue or board foreign key: collision
-- evidence remains meaningful when only one participant's board is present.
-- The saved replacement is a fallback; reads join by UID to follow later
-- public-ID changes for locally available issues.
CREATE TABLE issue_reference_candidates (
    marker_id TEXT NOT NULL REFERENCES issue_reference_markers(id),
    issue_uid BLOB NOT NULL CHECK (
        typeof(issue_uid) = 'blob' AND length(issue_uid) = 16
        AND issue_uid <> zeroblob(16)
    ),
    board_id TEXT NOT NULL CHECK (board_id <> ''),
    replacement_id TEXT NOT NULL,
    PRIMARY KEY (marker_id, issue_uid)
);

CREATE INDEX issue_reference_candidates_board
ON issue_reference_candidates(board_id, marker_id);

-- Protect the store-wide public namespace at every issue write boundary,
-- including restore and clone, rather than relying only on allocation.
-- +goose StatementBegin
CREATE TRIGGER issues_reject_retired_id_insert
BEFORE INSERT ON issues
WHEN EXISTS (SELECT 1 FROM issue_reference_markers WHERE id = NEW.id)
BEGIN
    SELECT RAISE(ABORT, 'issue ID is a retired reference');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER issues_reject_retired_id_update
BEFORE UPDATE OF id ON issues
WHEN EXISTS (SELECT 1 FROM issue_reference_markers WHERE id = NEW.id)
BEGIN
    SELECT RAISE(ABORT, 'issue ID is a retired reference');
END;
-- +goose StatementEnd

-- Collision reconciliation renames participants before reserving their old
-- ID, within the same transaction. A marker cannot shadow a live issue.
-- +goose StatementBegin
CREATE TRIGGER issue_reference_markers_reject_live_id
BEFORE INSERT ON issue_reference_markers
WHEN EXISTS (SELECT 1 FROM issues WHERE id = NEW.id)
BEGIN
    SELECT RAISE(ABORT, 'reference marker ID belongs to an issue');
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER issue_reference_markers_reject_live_id;
DROP TRIGGER issues_reject_retired_id_update;
DROP TRIGGER issues_reject_retired_id_insert;
DROP TABLE issue_reference_candidates;
DROP TABLE issue_reference_markers;
