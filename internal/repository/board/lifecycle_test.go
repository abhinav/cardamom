package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
)

func TestRepositoryReleaseCommitsChangedStateOnce(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "release-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	recorder := record.NewRecorder(repository, repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Retain recovery State", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("author"),
		record.SetStateRequest{
			IssueID:    "release-1",
			Text:       "Continue from the diagnostic boundary.",
			NextAction: "Re-run the boundary probe.",
		},
	)
	require.NoError(t, err)

	claimAndRelease := func(actor string) {
		t.Helper()
		_, err = executor.ClaimIssue(
			t.Context(),
			issue.NewInvocation(actor),
			execution.ClaimIssueRequest{
				ID: "release-1", Assignee: actor,
			},
		)
		require.NoError(t, err)
		_, err = executor.ReleaseIssue(
			t.Context(),
			issue.NewInvocation(actor),
			execution.ReleaseIssueRequest{ID: "release-1"},
		)
		require.NoError(t, err)
	}
	claimAndRelease("first-worker")

	firstEntries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "release-1"},
	)
	require.NoError(t, err)
	require.Len(t, firstEntries, 1)
	assert.Equal(t, "state_snapshot", firstEntries[0].Kind)
	assert.Equal(t, new("author"), firstEntries[0].Author)
	assert.Equal(t, new("first-worker"), firstEntries[0].Committer)
	assert.Equal(t, new("Re-run the boundary probe."), firstEntries[0].NextAction)
	assert.Equal(t, new(int64(1_700_000_000)), firstEntries[0].Created)

	view, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "release-1"},
	)
	require.NoError(t, err)
	require.NotNil(t, view.Detail.State)
	assert.Equal(
		t,
		"Continue from the diagnostic boundary.",
		view.Detail.State.Body,
	)
	assert.Equal(t, "Re-run the boundary probe.", view.Detail.State.NextAction)
	assert.Equal(t, issue.NewActor("author"), view.Detail.State.Author)
	assert.Equal(
		t,
		new(time.Unix(1_700_000_000, 0).UTC()),
		view.Detail.State.UpdatedAt,
	)
	assert.Equal(
		t,
		&firstEntries[0].ID,
		view.Detail.State.SnapshotLogEntryID,
	)

	claimAndRelease("second-worker")
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "release-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, firstEntries, entries)
}

func TestRepositoryClaimsReleasesAndTransitionsLifecycle(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "life-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	requests := []planning.CreateIssueRequest{
		{Title: "Ready task", Type: "task", Priority: 1, Labels: []string{"backend"}},
		{Title: "Routine", Type: "routine", Priority: 0, Labels: []string{"backend"}},
		{Title: "Cancellation root", Type: "task", Priority: 2},
		{Title: "Cancellation dependent", Type: "task", Priority: 2, DependsOn: []string{"life-3"}},
	}
	for _, request := range requests {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}
	recorder := record.NewRecorder(repository, repository)
	_, err := recorder.SetState(
		t.Context(),
		issue.NewInvocation("planner"),
		record.SetStateRequest{
			IssueID:    "life-2",
			Text:       "Keep the routine recovery facts.",
			NextAction: "Run the scheduled maintenance cycle.",
		},
	)
	require.NoError(t, err)

	claimed, err := executor.ClaimNext(t.Context(), issue.NewInvocation("engineer"), execution.ClaimNextRequest{
		Assignee: "engineer", LabelsAll: []string{"backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, "life-1", claimed.Issue.Detail.Issue.ID)
	assert.Equal(t, "in_progress", claimed.Issue.Detail.Issue.Status)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("navigator"),
		record.SetStateRequest{
			IssueID: "life-1", Text: "Preserve this recovery position.",
		},
	)
	require.NoError(t, err)

	_, err = executor.ReleaseIssue(t.Context(), issue.NewInvocation("other"), execution.ReleaseIssueRequest{ID: "life-1"})
	require.ErrorContains(t, err, "belongs to engineer")
	released, err := executor.ReleaseIssue(t.Context(), issue.NewInvocation("engineer"), execution.ReleaseIssueRequest{ID: "life-1"})
	require.NoError(t, err)
	assert.Equal(t, "ready", released.Issue.Issue.Status)
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "life-1"},
	)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state_snapshot", entries[0].Kind)
	assert.Equal(t, new("navigator"), entries[0].Author)
	assert.Equal(t, new("engineer"), entries[0].Committer)

	routine, err := executor.ClaimIssue(t.Context(), issue.NewInvocation("caretaker"), execution.ClaimIssueRequest{
		ID: "life-2", Assignee: "caretaker",
	})
	require.NoError(t, err)
	assert.Equal(t, "life-2", routine.Issue.Detail.Issue.ID)
	require.NotNil(t, routine.Issue.Detail.State)
	assert.Equal(
		t,
		"Keep the routine recovery facts.",
		routine.Issue.Detail.State.Body,
	)
	assert.Equal(
		t,
		"Run the scheduled maintenance cycle.",
		routine.Issue.Detail.State.NextAction,
	)
	assert.Equal(t, issue.NewActor("planner"), routine.Issue.Detail.State.Author)
	assert.Nil(t, routine.Issue.Detail.State.SnapshotLogEntryID)

	closed, err := executor.CloseIssues(t.Context(), issue.NewInvocation("captain"), execution.CloseIssuesRequest{IDs: []string{"life-1"}})
	require.NoError(t, err)
	assert.Equal(t, "closed", closed.Issues[0].Issue.Status)
	closedView, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "life-1"},
	)
	require.NoError(t, err)
	assert.Nil(t, closedView.Detail.State)
	assert.Nil(t, closedView.Detail.Issue.State)
	entries, err = repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "life-1"},
	)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	reopened, err := executor.ReopenIssues(t.Context(), issue.NewInvocation("captain"), execution.ReopenIssuesRequest{IDs: []string{"life-1"}})
	require.NoError(t, err)
	assert.Equal(t, "ready", reopened.Issues[0].Issue.Issue.Status)

	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("root-author"),
		record.SetStateRequest{
			IssueID: "life-3", Text: "Root recovery position.",
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("dependent-author"),
		record.SetStateRequest{
			IssueID: "life-4", Text: "Dependent recovery position.",
		},
	)
	require.NoError(t, err)
	cancelled, err := executor.CancelIssues(t.Context(), issue.NewInvocation("captain"), execution.CancelIssuesRequest{Roots: []string{"life-3"}})
	require.NoError(t, err)
	assert.Equal(t, 1, cancelled.Requested)
	assert.Equal(t, 1, cancelled.Dependents)
	assert.Equal(t, []string{"life-3", "life-4"}, issueIDs(cancelled.Issues))
	for _, test := range []struct {
		id     string
		author string
	}{
		{id: "life-3", author: "root-author"},
		{id: "life-4", author: "dependent-author"},
	} {
		view, err := repository.ReadIssue(
			t.Context(),
			issue.ReadRequest{IssueID: test.id},
		)
		require.NoError(t, err)
		assert.Nil(t, view.Detail.State)
		entries, err := repository.ListLogEntries(
			t.Context(),
			issue.LogListRequest{IssueID: test.id},
		)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, new(test.author), entries[0].Author)
		assert.Equal(t, new("captain"), entries[0].Committer)
	}

	actors, err := repository.ListActors(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"captain",
		"caretaker",
		"dependent-author",
		"engineer",
		"navigator",
		"planner",
		"root-author",
	}, actors)
}

func TestRepositoryClaimNextMatchesAllAnyAndNoneLabels(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "claim-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	requests := []planning.CreateIssueRequest{
		{Title: "Open prerequisite", Type: "task", Priority: 2},
		{
			Title: "Blocked", Type: "task", Priority: 0,
			Labels: []string{"alpha", "blue"}, DependsOn: []string{"claim-1"},
		},
		{Title: "Missing all", Type: "task", Priority: 0, Labels: []string{"green"}},
		{Title: "Missing any", Type: "task", Priority: 1, Labels: []string{"alpha", "red"}},
		{Title: "Excluded by none", Type: "task", Priority: 1, Labels: []string{"alpha", "blue", "paused"}},
		{Title: "Selected first", Type: "task", Priority: 1, Labels: []string{"alpha", "green"}},
		{Title: "Selected later", Type: "task", Priority: 1, Labels: []string{"alpha", "blue"}},
	}
	for _, request := range requests {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}

	claimed, err := executor.ClaimNext(
		t.Context(),
		issue.NewInvocation("worker"),
		execution.ClaimNextRequest{
			Assignee:   "worker",
			LabelsAll:  []string{"alpha"},
			LabelsAny:  []string{"blue", "green"},
			LabelsNone: []string{"paused"},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "claim-6", claimed.Issue.Detail.Issue.ID)
}

func TestRepositoryResolvesCheckpointsWithoutApproverPolicy(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "gate-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	requests := []planning.CreateIssueRequest{
		{Title: "Passing gate", Type: "checkpoint", Priority: 1},
		{Title: "After pass", Type: "task", Priority: 2, DependsOn: []string{"gate-1"}},
		{Title: "Failing gate", Type: "checkpoint", Priority: 1},
		{Title: "After fail", Type: "task", Priority: 2, DependsOn: []string{"gate-3"}},
	}
	for _, request := range requests {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}
	recorder := record.NewRecorder(repository, repository)
	for _, state := range []struct {
		id   string
		body string
	}{
		{id: "gate-1", body: "Passing gate context."},
		{id: "gate-3", body: "Failing gate context."},
		{id: "gate-4", body: "Dependent context."},
	} {
		_, err := recorder.SetState(
			t.Context(),
			issue.NewInvocation("author"),
			record.SetStateRequest{IssueID: state.id, Text: state.body},
		)
		require.NoError(t, err)
	}

	approved, err := executor.ApproveCheckpoint(t.Context(), issue.NewInvocation("reviewer"), execution.CheckpointRequest{
		IssueID: "gate-1", Reason: "Approved after inspection.",
	})
	require.NoError(t, err)
	require.NotNil(t, approved.Issue)
	assert.Equal(t, "closed", approved.Issue.Status)
	assert.Equal(t, "approved", approved.Decision.Outcome)
	assert.Equal(t, "Approved after inspection.", approved.Decision.Reason)
	entries, err := repository.ListLogEntries(t.Context(), issue.LogListRequest{IssueID: "gate-1"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "state_snapshot", entries[0].Kind)
	assert.Equal(t, new("author"), entries[0].Author)
	assert.Equal(t, new("reviewer"), entries[0].Committer)
	approvedView, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "gate-1"})
	require.NoError(t, err)
	assert.Equal(t, &approved.Decision, approvedView.Detail.CheckpointDecision)
	assert.Nil(t, approvedView.Detail.State)

	denied, err := executor.DenyCheckpoint(t.Context(), issue.NewInvocation("reviewer"), execution.CheckpointRequest{
		IssueID: "gate-3", Reason: "Rejected after inspection.",
	})
	require.NoError(t, err)
	assert.Equal(t, "denied", denied.Decision.Outcome)
	assert.Equal(t, "Rejected after inspection.", denied.Decision.Reason)
	assert.Equal(t, []string{"gate-3", "gate-4"}, issueIDs(denied.Cancelled))
	entries, err = repository.ListLogEntries(t.Context(), issue.LogListRequest{IssueID: "gate-3"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, new("author"), entries[0].Author)
	assert.Equal(t, new("reviewer"), entries[0].Committer)
	deniedView, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "gate-3"})
	require.NoError(t, err)
	assert.Equal(t, &denied.Decision, deniedView.Detail.CheckpointDecision)
	assert.Nil(t, deniedView.Detail.State)
	dependentView, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "gate-4"},
	)
	require.NoError(t, err)
	assert.Nil(t, dependentView.Detail.State)
	entries, err = repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "gate-4"},
	)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, new("reviewer"), entries[0].Committer)

	checkpoints, err := repository.ListActionableCheckpoints(t.Context())
	require.NoError(t, err)
	assert.Empty(t, checkpoints)
}

func issueIDs(values []issue.Issue) []string {
	ids := make([]string, len(values))
	for index, value := range values {
		ids[index] = value.ID
	}
	return ids
}
