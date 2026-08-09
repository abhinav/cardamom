package selection

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.uber.org/mock/gomock"
)

func TestResolver_Resolve_precedence(t *testing.T) {
	defaultBoard := testBoard(t, "board-default", "Default")
	secondBoard := testBoard(t, "board-second", "Second")
	boards := []*board.State{defaultBoard, secondBoard}
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().List(gomock.Any()).Return(boards, nil).Times(2)
	catalog.EXPECT().Get(gomock.Any(), secondBoard.ID()).Return(secondBoard, nil)
	catalog.EXPECT().Get(gomock.Any(), defaultBoard.ID()).Return(defaultBoard, nil)
	binding := NewMockBinding(gomock.NewController(t))
	binding.EXPECT().Read().Return(secondBoard.ID(), nil)
	issues := NewMockIssueLocator(gomock.NewController(t))
	issues.EXPECT().BoardForIssue(gomock.Any(), "issue-default").Return(defaultBoard.ID(), nil)
	resolver := NewResolver(catalog, binding, issues)
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
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().List(gomock.Any()).Return([]*board.State{defaultBoard, secondBoard}, nil)
	issues := NewMockIssueLocator(gomock.NewController(t))
	issues.EXPECT().BoardForIssue(gomock.Any(), "issue-second").Return(secondBoard.ID(), nil)
	resolver := NewResolver(catalog, NewMockBinding(gomock.NewController(t)), issues)
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
	issues := NewMockIssueLocator(gomock.NewController(t))
	issues.EXPECT().BoardForIssue(gomock.Any(), "issue-default").Return(defaultBoard.ID(), nil)
	issues.EXPECT().BoardForIssue(gomock.Any(), "issue-second").Return(secondBoard.ID(), nil)
	resolver := NewResolver(
		NewMockCatalog(gomock.NewController(t)),
		NewMockBinding(gomock.NewController(t)),
		issues,
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
	issues := NewMockIssueLocator(gomock.NewController(t))
	issues.EXPECT().BoardForIssue(gomock.Any(), "missing").Return(board.ID(""), ErrIssueNotFound)
	resolver := NewResolver(
		NewMockCatalog(gomock.NewController(t)),
		NewMockBinding(gomock.NewController(t)),
		issues,
	)

	_, err := resolver.Resolve(t.Context(), Request{IssueIDs: []string{"missing"}})

	assert.ErrorIs(t, err, ErrIssueNotFound)
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, "issue not found")
}

func TestResolver_Resolve_requiresUnambiguousAmbientBoard(t *testing.T) {
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().Sole(gomock.Any()).Return(nil, errkind.Errorf(
		errkind.Conflict, "board selection is ambiguous",
	))
	binding := NewMockBinding(gomock.NewController(t))
	binding.EXPECT().Read().Return(board.ID(""), ErrBindingNotFound)
	resolver := NewResolver(catalog, binding, NewMockIssueLocator(gomock.NewController(t)))

	_, err := resolver.Resolve(t.Context(), Request{})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, "board selection is ambiguous")
}

func TestResolver_Resolve_reportsMissingAmbientBoard(t *testing.T) {
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().Sole(gomock.Any()).Return(nil, errkind.Errorf(errkind.NotFound, "board not found"))
	binding := NewMockBinding(gomock.NewController(t))
	binding.EXPECT().Read().Return(board.ID(""), ErrBindingNotFound)
	resolver := NewResolver(catalog, binding, NewMockIssueLocator(gomock.NewController(t)))

	_, err := resolver.Resolve(t.Context(), Request{})

	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, "board not found")
}

func TestResolver_Resolve_reportsExplicitBoardNotFound(t *testing.T) {
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().List(gomock.Any()).Return([]*board.State{}, nil)
	resolver := NewResolver(
		catalog,
		NewMockBinding(gomock.NewController(t)),
		NewMockIssueLocator(gomock.NewController(t)),
	)
	selector := testBoardSelector(t, "Missing")

	_, err := resolver.Resolve(t.Context(), Request{Selector: &selector})

	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.EqualError(t, err, `board "Missing" not found`)
}

func TestResolver_Resolve_reportsAmbiguousExplicitBoard(t *testing.T) {
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().List(gomock.Any()).Return([]*board.State{
		testBoard(t, "board-one", "Shared"),
		testBoard(t, "board-two", "Shared"),
	}, nil)
	resolver := NewResolver(
		catalog,
		NewMockBinding(gomock.NewController(t)),
		NewMockIssueLocator(gomock.NewController(t)),
	)
	selector := testBoardSelector(t, "Shared")

	_, err := resolver.Resolve(t.Context(), Request{Selector: &selector})

	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.EqualError(t, err, `board "Shared" is ambiguous; use an ID`)
}

func TestResolver_Use_persistsCheckoutBinding(t *testing.T) {
	state := testBoard(t, "board-one", "Development")
	catalog := NewMockCatalog(gomock.NewController(t))
	catalog.EXPECT().List(gomock.Any()).Return([]*board.State{state}, nil)
	binding := NewMockBinding(gomock.NewController(t))
	binding.EXPECT().Write(state.ID()).Return(nil)
	resolver := NewResolver(
		catalog,
		binding,
		NewMockIssueLocator(gomock.NewController(t)),
	)

	selected, err := resolver.Use(
		t.Context(),
		testBoardSelector(t, "Development"),
	)

	require.NoError(t, err)
	assert.Same(t, state, selected)
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
