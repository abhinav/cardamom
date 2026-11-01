package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullableMigrationPreservesPinsAndAllowsPositionTies(t *testing.T) {
	db := populatedStoreBeforeNullableFields(t)
	provider, err := newStoreMigrationProvider(db)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 20261029220000)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO board_pins (board_id, issue_uid, position)
SELECT board_id, uid, 1 FROM issues WHERE id = 'card-two'`)
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), `
SELECT count(*) FROM board_pins WHERE board_id = 'board' AND position = 1`).Scan(&count))
	assert.Equal(t, 2, count)
	_, err = db.ExecContext(t.Context(), `
INSERT INTO board_pins (board_id, issue_uid, position)
SELECT board_id, uid, 2 FROM issues WHERE id = 'card-two'`)
	assert.ErrorContains(t, err, "UNIQUE constraint failed")
	_, err = db.ExecContext(t.Context(), "UPDATE board_pins SET position = 0")
	assert.ErrorContains(t, err, "CHECK constraint failed")
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM pragma_foreign_key_check").Scan(&count))
	assert.Zero(t, count)
}
