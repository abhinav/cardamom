package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenRejectsForeignKeyCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "foreign-key.db")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	require.NoError(t, persistence.Close())

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(0)")
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO issue_labels(board_id, issue_id, label)
		VALUES ('missing-board', 'missing-issue', 'broken')
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
			id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES
			('an-first', 'board-first', 'First issue', 'task', 'open', 2, 1, 1),
			('an-second', 'board-second', 'Second issue', 'task', 'open', 2, 1, 1);
		INSERT INTO dependencies(board_id, issue_id, prerequisite_id)
		VALUES ('board-first', 'an-first', 'an-second');
	`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err = Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "foreign keys")
}

func TestOpenRejectsIssueSearchProjectionCorruption(t *testing.T) {
	path := openSearchVerificationStore(t)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`
DELETE FROM issue_search_documents
WHERE issue_id = 'an-issue' AND field = 'title'
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
    WHERE issue_id = 'an-issue' AND field = 'title'
)
`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	persistence, err := Open(t.Context(), Config{Path: path})
	assert.Nil(t, persistence)
	assert.ErrorContains(t, err, "issue search index")
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
INSERT INTO issues(
    id, board_id, title, kind, lifecycle, priority, created_at, updated_at
) VALUES ('an-issue', 'board', 'Indexed title', 'task', 'open', 2, 1, 1)
`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	require.NoError(t, persistence.Close())
	return path
}
