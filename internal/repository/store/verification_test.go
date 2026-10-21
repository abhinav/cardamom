package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func TestOpenRejectsForeignKeyCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foreign-key.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO issue_labels(board_id, issue_uid, label)
		VALUES ('missing-board', randomblob(16), 'broken')
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "foreign keys")
}

func TestOpenRejectsProjectionRevisionBeyondStoreHead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store-state.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO projects(id, name, created_at)
		VALUES ('project', 'Project', 1);
		INSERT INTO boards(id, project_id, name, created_at, revision)
		VALUES ('board', 'project', 'Board', 1, 1);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "projection revisions")
}

func TestOpenRejectsBoardWithoutReplicaIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board-replica.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO projects(id, name, created_at)
		VALUES ('project', 'Project', 1);
		INSERT INTO boards(id, project_id, name, created_at)
		VALUES ('board', 'project', 'Board', 1);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "board replica identities")
}

func TestOpenRejectsCrossBoardRelationshipCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cross-board-dependency.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO projects(id, name, created_at)
		VALUES ('project', 'Project', 1);
		INSERT INTO boards(id, project_id, name, created_at)
		VALUES
			('board-first', 'project', 'First', 1),
			('board-second', 'project', 'Second', 1);
		INSERT INTO issues(
			uid, id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES
			(randomblob(16), 'an-first', 'board-first', 'First issue', 'task', 'open', 2, 1, 1),
			(randomblob(16), 'an-second', 'board-second', 'Second issue', 'task', 'open', 2, 1, 1);
		INSERT INTO dependencies(board_id, issue_uid, prerequisite_uid)
		VALUES (
			'board-first',
			(SELECT uid FROM issues WHERE id = 'an-first'),
			(SELECT uid FROM issues WHERE id = 'an-second')
		);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "foreign keys")
}

func TestOpenRejectsInvalidIssueUID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-issue-uid.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path+"?_pragma=ignore_check_constraints(1)")
	require.NoError(t, err)
	_, err = db.Exec(`
		DROP TRIGGER issues_reject_uid_update;
		INSERT INTO projects(id, name, created_at) VALUES ('project', 'Project', 1);
		INSERT INTO boards(id, project_id, name, created_at)
		VALUES ('board', 'project', 'Board', 1);
		INSERT INTO issues(
			uid, id, board_id, title, kind, lifecycle, priority, created_at, updated_at
		) VALUES (zeroblob(16), 'broken', 'board', 'Broken', 'task', 'open', 2, 1, 1);
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "CHECK constraint failed in issues")
}

func TestOpenRejectsIssueSearchProjectionCorruption(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
DELETE FROM issue_search_documents
WHERE issue_uid = (SELECT uid FROM issues WHERE id = 'an-issue')
    AND field = 'title'
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "issue search documents")
}

func TestOpenRejectsIssueSearchIndexCorruption(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
DELETE FROM issue_search_fts
WHERE rowid = (
    SELECT rowid
    FROM issue_search_documents
    WHERE issue_uid = (SELECT uid FROM issues WHERE id = 'an-issue')
        AND field = 'title'
)
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "issue search index")
}

func TestOpenRejectsBoardMutationPayloadCorruption(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
UPDATE board_mutation_parts
SET bytes = CAST(X'00' || substr(bytes, 2) AS BLOB)
WHERE board_id = 'board' AND sequence = 1 AND part_index = 0
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "board mutation history")
}

func TestOpenAcceptsDeduplicatedBoardMutationBlobReferences(t *testing.T) {
	path := openAttachmentHistoryVerificationStore(t)
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())
}

func TestOpenRejectsMissingBoardMutationBlobReference(t *testing.T) {
	path := openAttachmentHistoryVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM board_mutation_blob_refs`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "attachment blob reference")
	assert.ErrorContains(t, err, "is missing")
}

func TestOpenRejectsExtraneousBoardMutationBlobReference(t *testing.T) {
	path := openAttachmentHistoryVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
INSERT INTO attachment_blobs(digest, size_bytes)
VALUES ('sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 7);
INSERT INTO board_mutation_blob_refs(
    board_id, writer_uid, sequence, digest, size_bytes
)
SELECT board_id, writer_uid, sequence,
    'sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb', 7
FROM board_mutation_commits
WHERE board_id = 'board' AND sequence = 1;
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "has no attachment add event")
}

func TestVerifyRejectsBoardMutationBlobReferenceWrongSize(t *testing.T) {
	path := openAttachmentHistoryVerificationStore(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	require.NoError(t, err)
	defer func() { assert.NoError(t, db.Close()) }()
	_, err = db.Exec(`UPDATE board_mutation_blob_refs SET size_bytes = 5`)
	require.NoError(t, err)

	err = verifyBoardMutationHistory(t.Context(), query.New(db))
	assert.ErrorContains(t, err, "has size 5; want 4")
}

func TestOpenRejectsMissingMutationFrontierTip(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	require.NoError(t, err)
	var markerBytes []byte
	require.NoError(t, db.QueryRow(`
SELECT marker_bytes
FROM board_mutation_commits
WHERE board_id = 'board' AND sequence = 1
`).Scan(&markerBytes))
	invalidTip := `[{"hash":"` +
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, sha256.Size)) +
		`","sequence":1,"writer_uid":"` +
		base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 16)) + `"}]`
	markerBytes = []byte(strings.Replace(
		string(markerBytes), `"frontier":[]`, `"frontier":`+invalidTip, 1,
	))
	markerHash := sha256.Sum256(markerBytes)
	_, err = db.Exec(`
UPDATE board_mutation_tips
SET marker_hash = ?
WHERE board_id = 'board' AND sequence = 1
`, markerHash[:])
	require.NoError(t, err)
	_, err = db.Exec(`
UPDATE board_mutation_commits
SET marker_bytes = ?, marker_hash = ?
WHERE board_id = 'board' AND sequence = 1
`, markerBytes, markerHash[:])
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "does not identify retained history")
}

func TestOpenRejectsBoardWithoutLocalMutationTip(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM board_mutation_tips WHERE board_id = 'board'`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "without local history")
}

func openSearchVerificationStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issue-search.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects(id, name, created_at)
VALUES ('project', 'Project', 1);
INSERT INTO boards(id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 1);
INSERT INTO board_replica_identities(board_id, writer_uid)
VALUES ('board', X'01010101010101010101010101010101');
INSERT INTO issues(
    uid, id, board_id, title, kind, lifecycle, priority, created_at, updated_at
) VALUES (randomblob(16), 'an-issue', 'board', 'Indexed title',
    'task', 'open', 2, 1, 1)
`)
	require.NoError(t, err)
	initializeTestBoardHistory(t, change, "board")
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	require.NoError(t, persistence.Close())
	return path
}

func openAttachmentHistoryVerificationStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "attachment-history.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects(id, name, created_at)
VALUES ('project', 'Project', 1);
INSERT INTO boards(id, project_id, name, created_at)
VALUES ('board', 'project', 'Board', 1);
INSERT INTO board_replica_identities(board_id, writer_uid)
VALUES ('board', X'01010101010101010101010101010101');
INSERT INTO attachment_blobs(digest, size_bytes)
VALUES ('sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa', 4);
INSERT INTO attachments(
    board_id, id, blob_digest, blob_size_bytes, filename, media_type,
    lifecycle, created_actor, created_at, created_revision
) VALUES (
    'board', 'att_aaaaaaaaaaaaaaaaaaaaaaaaaa',
    'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    4, 'evidence.txt', 'text/plain', 'active', 'alice', 1, 1
), (
    'board', 'att_bbbbbbbbbbbbbbbbbbbbbbbbba',
    'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    4, 'duplicate.txt', 'text/plain', 'active', 'alice', 1, 1
);
`)
	require.NoError(t, err)
	initializeTestBoardHistory(t, change, "board")
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	require.NoError(t, persistence.Close())
	return path
}

func initializeTestBoardHistory(t *testing.T, db query.DBTX, boardIDs ...string) {
	t.Helper()
	for _, rawBoardID := range boardIDs {
		boardID, err := board.NewID(rawBoardID)
		require.NoError(t, err)
		require.NoError(t, repositoryhistory.InitializeV1(
			t.Context(), db, boardID, time.Unix(1, 0).UTC(),
		))
	}
}
