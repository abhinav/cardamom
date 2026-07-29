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
