package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenMigratesAttachmentSchema(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "attachments.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	rows, err := view.QueryContext(t.Context(), `
		SELECT name
		FROM sqlite_schema
		WHERE type = 'table'
			AND name IN ('attachment_blobs', 'attachments', 'attachment_uploads')
		ORDER BY name
	`)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()

	var tables []string
	for rows.Next() {
		var table string
		require.NoError(t, rows.Scan(&table))
		tables = append(tables, table)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{
		"attachment_blobs",
		"attachment_uploads",
		"attachments",
	}, tables)
}

func TestAttachmentSchemaEnforcesPersistenceInvariants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "attachments.sqlite3")
	persistence, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()

	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at)
		VALUES ('project', 'Project', 1);
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES
			('board-one', 'project', 'One', 1),
			('board-two', 'project', 'Two', 1);
		UPDATE store_state SET current_revision = 2 WHERE singleton = 1;
		INSERT INTO issues (
			id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES
			('issue-one', 'board-one', 'One', 'task', 'open', 2, 1, 1),
			('issue-two', 'board-two', 'Two', 'task', 'open', 2, 1, 1);
	`)
	require.NoError(t, err)

	digest := "sha256:" + strings.Repeat("0", 64)
	const attachmentID = "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO attachment_blobs (digest, size_bytes) VALUES (?, 42)
	`, digest)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO attachments (
			board_id, id, origin_issue_id, blob_digest, blob_size_bytes,
			filename, media_type, lifecycle,
			created_actor, created_at, created_revision
		) VALUES (
			'board-one', ?, 'issue-one', ?, 42,
			'artifact.txt', 'text/plain; charset=utf-8', 'active',
			'captain', 10, 1
		)
	`, attachmentID, digest)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO attachment_uploads (
			id, board_id, origin_issue_id, filename,
			expected_size_bytes, expected_digest, actor, state,
			accepted_offset, expires_at
		) VALUES (
			'upload-one', 'board-one', 'issue-one', 'artifact.txt',
			42, ?, 'captain', 'active', 0, 100
		)
	`, digest)
	require.NoError(t, err)

	t.Run("BlobDescriptor", func(t *testing.T) {
		_, err := change.ExecContext(t.Context(), `
			INSERT INTO attachment_blobs (digest, size_bytes) VALUES (?, 0)
		`, "sha256:"+strings.Repeat("A", 64))
		assert.Error(t, err)

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachment_blobs SET size_bytes = 41 WHERE digest = ?
		`, digest)
		assert.ErrorContains(t, err, "attachment blob descriptors are immutable")
	})

	t.Run("BoardScopedOrigin", func(t *testing.T) {
		_, err := change.ExecContext(t.Context(), `
			INSERT INTO attachments (
				board_id, id, origin_issue_id, blob_digest, blob_size_bytes,
				filename, media_type, lifecycle,
				created_actor, created_at, created_revision
			) VALUES (
				'board-one', 'att_bbbbbbbbbbbbbbbbbbbbbbbbba', 'issue-two', ?, 42,
				'artifact.txt', 'text/plain', 'active', 'captain', 10, 1
			)
		`, digest)
		assert.Error(t, err)
	})

	t.Run("UploadProgressAndReceipt", func(t *testing.T) {
		_, err := change.ExecContext(t.Context(), `
			UPDATE attachment_uploads SET accepted_offset = 42
			WHERE id = 'upload-one'
		`)
		require.NoError(t, err)

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachment_uploads SET accepted_offset = 41
			WHERE id = 'upload-one'
		`)
		assert.ErrorContains(t, err, "attachment upload offset cannot move backward")

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachment_uploads
			SET state = 'committed', attachment_id = ?
			WHERE id = 'upload-one'
		`, attachmentID)
		require.NoError(t, err)

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachment_uploads SET expires_at = 101
			WHERE id = 'upload-one'
		`)
		assert.ErrorContains(t, err, "terminal attachment upload receipts are immutable")

		_, err = change.ExecContext(t.Context(), `
			INSERT INTO attachment_uploads (
				id, board_id, filename, actor, state,
				accepted_offset, expires_at, attachment_id
			) VALUES (
				'cross-board', 'board-two', 'artifact.txt', 'captain',
				'committed', 42, 100, ?
			)
		`, attachmentID)
		assert.Error(t, err)
	})

	t.Run("AttachmentTombstone", func(t *testing.T) {
		_, err := change.ExecContext(t.Context(), `
			UPDATE attachments SET lifecycle = 'removed'
			WHERE board_id = 'board-one' AND id = ?
		`, attachmentID)
		assert.Error(t, err)

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachments
			SET lifecycle = 'removed', removed_actor = 'captain',
				removed_at = 20, removed_revision = 2
			WHERE board_id = 'board-one' AND id = ?
		`, attachmentID)
		require.NoError(t, err)

		_, err = change.ExecContext(t.Context(), `
			UPDATE attachments
			SET lifecycle = 'active', removed_actor = NULL,
				removed_at = NULL, removed_revision = NULL
			WHERE board_id = 'board-one' AND id = ?
		`, attachmentID)
		assert.ErrorContains(t, err, "attachment tombstones are immutable")
	})

	require.NoError(t, change.Commit())
	require.NoError(t, persistence.Close())

	verified, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	assert.NoError(t, verified.Close())
}
