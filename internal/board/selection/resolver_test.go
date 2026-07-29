package selection

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestResolver_Resolve_precedence(t *testing.T) {
	defaultBoard := testBoard(t, "board-default", "Default")
	secondBoard := testBoard(t, "board-second", "Second")
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{defaultBoard, secondBoard}},
		&fakeBinding{boardID: secondBoard.ID()},
		fakeIssueBoards{"issue-default": defaultBoard.ID()},
	)
	selector := testBoardSelector(t, "Default")

	selected, err := resolver.Resolve(t.Context(), Request{Selector: &selector})
	require.NoError(t, err)
	assert.Same(t, defaultBoard, selected)

	selected, err = resolver.Resolve(t.Context(), Request{})
	require.NoError(t, err)
	assert.Same(t, secondBoard, selected)

	selected, err = resolver.Resolve(t.Context(), Request{
		Selector: &selector,
		IssueIDs: []string{"issue-default"},
	})
	require.NoError(t, err)
	assert.Same(t, defaultBoard, selected)
}

func TestResolver_Resolve_rejectsIssueBoardMismatch(t *testing.T) {
	defaultBoard := testBoard(t, "board-default", "Default")
	secondBoard := testBoard(t, "board-second", "Second")
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{defaultBoard, secondBoard}},
		&fakeBinding{},
		fakeIssueBoards{"issue-second": secondBoard.ID()},
	)
	selector := testBoardSelector(t, "Default")

	_, err := resolver.Resolve(t.Context(), Request{
		Selector: &selector,
		IssueIDs: []string{"issue-second"},
	})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(
		t,
		err,
		`issue "issue-second" belongs to board "board-second", not selected board "board-default"`,
	)
}

func TestResolver_Resolve_rejectsIssuesFromMultipleBoards(t *testing.T) {
	defaultBoard := testBoard(t, "board-default", "Default")
	secondBoard := testBoard(t, "board-second", "Second")
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{defaultBoard, secondBoard}},
		&fakeBinding{},
		fakeIssueBoards{
			"issue-default": defaultBoard.ID(),
			"issue-second":  secondBoard.ID(),
		},
	)

	_, err := resolver.Resolve(t.Context(), Request{
		IssueIDs: []string{"issue-default", "issue-second"},
	})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(
		t,
		err,
		`issue IDs belong to multiple boards: "board-default" and "board-second"`,
	)
}

func TestResolver_Resolve_reportsMissingIssue(t *testing.T) {
	resolver := NewResolver(&fakeCatalog{}, &fakeBinding{}, fakeIssueBoards{})

	_, err := resolver.Resolve(t.Context(), Request{IssueIDs: []string{"missing"}})

	assert.ErrorIs(t, err, ErrIssueNotFound)
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, "issue not found")
}

func TestResolver_Resolve_requiresUnambiguousAmbientBoard(t *testing.T) {
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{
			testBoard(t, "board-one", "First"),
			testBoard(t, "board-two", "Second"),
		}},
		&fakeBinding{},
		fakeIssueBoards{},
	)

	_, err := resolver.Resolve(t.Context(), Request{})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, "board selection is ambiguous")
}

func TestResolver_Resolve_reportsMissingAmbientBoard(t *testing.T) {
	resolver := NewResolver(&fakeCatalog{}, &fakeBinding{}, fakeIssueBoards{})

	_, err := resolver.Resolve(t.Context(), Request{})

	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, "board not found")
}

func TestResolver_Resolve_reportsExplicitBoardNotFound(t *testing.T) {
	resolver := NewResolver(&fakeCatalog{}, &fakeBinding{}, fakeIssueBoards{})
	selector := testBoardSelector(t, "Missing")

	_, err := resolver.Resolve(t.Context(), Request{Selector: &selector})

	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, `board "Missing" not found`)
}

func TestResolver_Resolve_reportsAmbiguousExplicitBoard(t *testing.T) {
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{
			testBoard(t, "board-one", "Shared"),
			testBoard(t, "board-two", "Shared"),
		}},
		&fakeBinding{},
		fakeIssueBoards{},
	)
	selector := testBoardSelector(t, "Shared")

	_, err := resolver.Resolve(t.Context(), Request{Selector: &selector})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, `board "Shared" is ambiguous; use an ID`)
}

func TestResolver_Use_persistsCheckoutBinding(t *testing.T) {
	state := testBoard(t, "board-one", "Development")
	binding := &fakeBinding{}
	resolver := NewResolver(
		&fakeCatalog{boards: []*board.State{state}},
		binding,
		fakeIssueBoards{},
	)

	selected, err := resolver.Use(
		t.Context(),
		testBoardSelector(t, "Development"),
	)

	require.NoError(t, err)
	assert.Same(t, state, selected)
	assert.Equal(t, state.ID(), binding.written)
}

type fakeCatalog struct {
	boards []*board.State
}

func (f *fakeCatalog) List(context.Context) ([]*board.State, error) {
	return f.boards, nil
}

func (f *fakeCatalog) Get(_ context.Context, id board.ID) (*board.State, error) {
	for _, state := range f.boards {
		if state.ID() == id {
			return state, nil
		}
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found")
}

func (f *fakeCatalog) Sole(context.Context) (*board.State, error) {
	switch len(f.boards) {
	case 0:
		return nil, errkind.Errorf(errkind.NotFound, "board not found")
	case 1:
		return f.boards[0], nil
	default:
		return nil, errkind.Errorf(errkind.Conflict, "board selection is ambiguous")
	}
}

type fakeBinding struct {
	boardID board.ID
	written board.ID
}

func (f *fakeBinding) Read() (board.ID, error) {
	if f.boardID == "" {
		return "", ErrBindingNotFound
	}
	return f.boardID, nil
}

func (f *fakeBinding) Write(id board.ID) error {
	f.written = id
	return nil
}

type fakeIssueBoards map[string]board.ID

func (f fakeIssueBoards) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := f[issueID]
	if !ok {
		return "", ErrIssueNotFound
	}
	return boardID, nil
}

func testBoard(t *testing.T, id, name string) *board.State {
	t.Helper()
	boardID, err := board.NewID(id)
	require.NoError(t, err)
	state, err := board.Load(board.Snapshot{
		ID:        boardID,
		ProjectID: "project-one",
		Name:      name,
		Created:   time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

func testBoardSelector(t *testing.T, value string) board.Selector {
	t.Helper()
	selector, err := board.NewSelector(value)
	require.NoError(t, err)
	return selector
}
