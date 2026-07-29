package boardscope

import (
	"context"
	"testing"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
)

func TestResolverBoards(t *testing.T) {
	t.Parallel()

	alpha := testBoard(t, "board-alpha")
	beta := testBoard(t, "board-beta")
	resolver := New(testCatalog{alpha, beta}, testIssueLocator{})

	one, err := resolver.Boards(t.Context(), &privatev1.BoardScope{
		Selection: &privatev1.BoardScope_BoardId{BoardId: alpha.ID().String()},
	})
	require.NoError(t, err)
	assert.Equal(t, []*board.State{alpha}, one)

	all, err := resolver.Boards(t.Context(), &privatev1.BoardScope{
		Selection: &privatev1.BoardScope_AllBoards{AllBoards: &privatev1.AllBoards{}},
	})
	require.NoError(t, err)
	assert.Equal(t, []*board.State{alpha, beta}, all)

	_, err = resolver.Boards(t.Context(), nil)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = resolver.Boards(t.Context(), &privatev1.BoardScope{})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestResolverIssueOwnership(t *testing.T) {
	t.Parallel()

	alpha := testBoard(t, "board-alpha")
	beta := testBoard(t, "board-beta")
	resolver := New(
		testCatalog{alpha, beta},
		testIssueLocator{"an-alpha": alpha.ID(), "an-beta": beta.ID()},
	)

	owner, err := resolver.BoardForIssue(t.Context(), "an-alpha")
	require.NoError(t, err)
	assert.Equal(t, alpha, owner)
	assert.NoError(t, resolver.RequireIssueBoard(t.Context(), "an-alpha", alpha.ID()))
	assert.Equal(
		t,
		errkind.Conflict,
		errkind.Of(resolver.RequireIssueBoard(t.Context(), "an-beta", alpha.ID())),
	)
	_, err = resolver.BoardForIssue(t.Context(), "")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

type testCatalog []*board.State

func (c testCatalog) Get(_ context.Context, id board.ID) (*board.State, error) {
	for _, board := range c {
		if board.ID() == id {
			return board, nil
		}
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found: %s", id)
}

func (c testCatalog) List(context.Context) ([]*board.State, error) {
	return c, nil
}

type testIssueLocator map[string]board.ID

func (l testIssueLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l[issueID]
	if !ok {
		return "", errkind.Errorf(errkind.NotFound, "issue not found: %s", issueID)
	}
	return boardID, nil
}

func testBoard(t *testing.T, id string) *board.State {
	t.Helper()
	board, err := board.Load(board.Snapshot{
		ID: board.ID(id), ProjectID: "project-test", Name: id,
		Created: time.Unix(1784376000, 0).UTC(),
	})
	require.NoError(t, err)
	return board
}
