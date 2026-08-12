package board

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepositoryPinsPreserveOrderAndAdmissionInvariant(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "First", Type: "task", Priority: 1},
		{Title: "Prerequisite", Type: "task", Priority: 1},
		{Title: "Blocked", Type: "task", Priority: 1, DependsOn: []string{"task-2"}},
	} {
		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			request,
		)
		require.NoError(t, err)
	}
	limit, err := domainboard.NewPinLimit(2)
	require.NoError(t, err)
	cursorBeforePin, err := repository.ReadChangeCursor(t.Context())
	require.NoError(t, err)

	first, err := repository.PinIssue(t.Context(), issue.ID("task-1"), limit)
	require.NoError(t, err)
	assert.True(t, first.Changed)
	assert.Equal(t, "First", first.Issue.Title)
	cursorAfterFirst, err := repository.ReadChangeCursor(t.Context())
	require.NoError(t, err)
	assert.Greater(t, cursorAfterFirst.Revision, cursorBeforePin.Revision)

	idempotent, err := repository.PinIssue(
		t.Context(), issue.ID("task-1"), domainboard.PinLimit(0),
	)
	require.NoError(t, err)
	assert.False(t, idempotent.Changed)
	cursorAfterNoOp, err := repository.ReadChangeCursor(t.Context())
	require.NoError(t, err)
	assert.Equal(t, cursorAfterFirst, cursorAfterNoOp)

	second, err := repository.PinIssue(t.Context(), issue.ID("task-3"), limit)
	require.NoError(t, err)
	assert.True(t, second.Changed)
	assert.Equal(t, "blocked", second.Issue.Status)
	_, err = repository.PinIssue(t.Context(), issue.ID("task-2"), limit)
	assert.ErrorIs(t, err, domainboard.ErrPinLimit)

	pins, err := repository.ListPins(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"task-1", "task-3"}, []string{
		pins[0].ID, pins[1].ID,
	})
	assert.Equal(t, "blocked", pins[1].Status)

	unpinned, err := repository.UnpinIssue(t.Context(), issue.ID("task-1"))
	require.NoError(t, err)
	assert.True(t, unpinned.Changed)
	repeated, err := repository.UnpinIssue(t.Context(), issue.ID("task-1"))
	require.NoError(t, err)
	assert.False(t, repeated.Changed)

	repinned, err := repository.PinIssue(t.Context(), issue.ID("task-1"), limit)
	require.NoError(t, err)
	assert.True(t, repinned.Changed)
	pins, err = repository.ListPins(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"task-3", "task-1"}, []string{
		pins[0].ID, pins[1].ID,
	})
}

func TestRepositoryPinsRejectMissingAndArchivedBoardMutation(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Pinned", Type: "task", Priority: 1},
	)
	require.NoError(t, err)
	limit, err := domainboard.NewPinLimit(1)
	require.NoError(t, err)
	_, err = repository.PinIssue(t.Context(), issue.ID("missing"), limit)
	assert.ErrorContains(t, err, "issue not found")
	_, err = repository.PinIssue(t.Context(), issue.ID("task-1"), limit)
	require.NoError(t, err)

	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(
		t.Context(),
		`UPDATE boards SET archived_at = 1700000001, archived_by = 'captain' WHERE id = 'board-test'`,
	)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	_, err = repository.PinIssue(t.Context(), issue.ID("task-1"), limit)
	assert.True(t, errors.Is(err, domainboard.ErrArchived))
	_, err = repository.UnpinIssue(t.Context(), issue.ID("task-1"))
	assert.True(t, errors.Is(err, domainboard.ErrArchived))
}

func TestRepositoryIssueContextIncludesPinnedIssueIDsAndTitles(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	for _, title := range []string{"Selected", "Pinned"} {
		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.CreateIssueRequest{Title: title, Type: "task", Priority: 1},
		)
		require.NoError(t, err)
	}
	limit, err := domainboard.NewPinLimit(1)
	require.NoError(t, err)
	_, err = repository.PinIssue(t.Context(), issue.ID("task-2"), limit)
	require.NoError(t, err)
	depth := 0

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{
		IssueID: "task-1", ContextDepth: &depth,
	})
	require.NoError(t, err)
	require.NotNil(t, view.Context)
	assert.Equal(t, []issue.PinnedIssue{{ID: "task-2", Title: "Pinned"}}, view.Context.Pins)
}
