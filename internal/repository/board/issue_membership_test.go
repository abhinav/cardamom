package board

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
)

func TestLocatorFindsStoreGlobalIssueBoard(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "member-", IDStrategy: "sequential",
	})
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-other', 'project-test', 'Other board', 1700000000)
	`)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issues (
			id, board_id, title, kind, lifecycle, priority, created_at, updated_at
		) VALUES ('other-issue', 'board-other', 'Other issue', 'task', 'open', 2, 1700000000, 1700000000)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	locator := NewLocator(repository.store)
	boardID, err := locator.BoardForIssue(t.Context(), "other-issue")
	require.NoError(t, err)
	assert.Equal(t, board.ID("board-other"), boardID)

	_, err = locator.BoardForIssue(t.Context(), "missing")
	assert.True(t, errors.Is(err, selection.ErrIssueNotFound))
}
