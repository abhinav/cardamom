package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepository_ReadIssueInventory(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID:  mustBoardID(t, "board-test"),
		IDPrefix: "info-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Active task", Type: "task", Priority: 1},
	)
	require.NoError(t, err)
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Closed task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Ready task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Waiting task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Prerequisite", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	_, err = planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Blocked task", Type: "task", Priority: 2,
			DependsOn: []string{"info-5"},
		},
	)
	require.NoError(t, err)
	_, err = executor.ClaimIssue(
		t.Context(),
		issue.NewInvocation("engineer"),
		execution.ClaimIssueRequest{ID: "info-1", Assignee: "engineer"},
	)
	require.NoError(t, err)
	_, err = executor.ClaimIssue(
		t.Context(),
		issue.NewInvocation("engineer"),
		execution.ClaimIssueRequest{ID: "info-4", Assignee: "engineer"},
	)
	require.NoError(t, err)
	waitingReason := "Awaiting review"
	_, err = executor.ReleaseIssue(
		t.Context(),
		issue.NewInvocation("engineer"),
		execution.ReleaseIssueRequest{ID: "info-4", WaitingReason: &waitingReason},
	)
	require.NoError(t, err)
	_, err = executor.CloseIssues(
		t.Context(),
		issue.NewInvocation("captain"),
		execution.CloseIssuesRequest{IDs: []string{"info-2"}},
	)
	require.NoError(t, err)

	view, err := repository.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	inventory, err := repository.ReadIssueInventory(t.Context(), view)
	require.NoError(t, err)

	assert.Equal(t, IssueInventory{
		Total: 6,
		ByStatus: []IssueStatusCount{
			{Status: issue.StatusReady, Count: 2},
			{Status: issue.StatusBlocked, Count: 1},
			{Status: issue.StatusInProgress, Count: 1},
			{Status: issue.StatusWaiting, Count: 1},
			{Status: issue.StatusClosed, Count: 1},
			{Status: issue.StatusCancelled, Count: 0},
		},
	}, inventory)
}
