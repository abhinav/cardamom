package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
)

func TestRepositoryPersistsRecordsAndCompletionValues(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "record-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	var err error
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Recovery record", Type: "task", Priority: 2, Labels: []string{"backend"},
	})
	require.NoError(t, err)

	_, err = recorder.SetState(t.Context(), issue.NewInvocation("captain"), record.SetStateRequest{
		IssueID: "record-1", Text: "Current state.",
	})
	require.NoError(t, err)
	_, err = recorder.AppendState(t.Context(), issue.NewInvocation("captain"), record.SetStateRequest{
		IssueID: "record-1", Text: "Next action.",
	})
	require.NoError(t, err)
	first, err := recorder.AddLogEntry(t.Context(), issue.NewInvocation("captain"), record.AddLogEntryRequest{
		IssueID: "record-1", Body: "First observation.",
	})
	require.NoError(t, err)
	second, err := recorder.AddLogEntry(t.Context(), issue.NewInvocation("engineer"), record.AddLogEntryRequest{
		IssueID: "record-1", Body: "Second observation.",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, first.LogEntry.ID)
	assert.NotEqual(t, first.LogEntry.ID, second.LogEntry.ID)
	assert.Contains(t, first.LogEntry.ID.String(), "log_")
	_, err = recorder.SetResult(t.Context(), issue.NewInvocation("captain"), record.SetResultRequest{
		IssueID: "record-1", Body: "Repository section complete.",
	})
	require.NoError(t, err)

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "record-1"})
	require.NoError(t, err)
	assert.Equal(t, new("Current state.\nNext action."), view.Detail.Issue.State)
	assert.Equal(t, issue.LogSummary{Count: 2, LatestID: &second.LogEntry.ID}, view.Detail.LogSummary)
	require.NotNil(t, view.Detail.CurrentResult)
	assert.Equal(t, "Repository section complete.", view.Detail.CurrentResult.Body)

	entries, err := repository.ListLogEntries(t.Context(), issue.LogListRequest{
		IssueID: "record-1", Reverse: true, Limit: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, []issue.LogEntry{{
		ID: second.LogEntry.ID, IssueID: "record-1", Kind: "post",
		Author: new("engineer"), Committer: new("engineer"),
		Body: "Second observation.", Created: new(int64(1_700_000_000)),
	}}, entries)

	result, err := repository.ReadResult(t.Context(), issue.ResultRequest{IssueID: "record-1"})
	require.NoError(t, err)
	assert.Equal(t, issue.Result{
		IssueID: "record-1", Title: "Recovery record", Body: "Repository section complete.",
	}, result)

	cursor, err := repository.ReadChangeCursor(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(6), cursor.Revision)
	ids, err := repository.ListIssueIDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"record-1"}, ids)
	labels, err := repository.ListLabels(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"backend"}, labels)
	actors, err := repository.ListActors(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"captain", "engineer"}, actors)
}

func TestRepositoryCommitsEachMaterialStateVersionOnce(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "state-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "State history", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("author"),
		record.SetStateRequest{
			IssueID: "state-1", Text: "Current finding.",
		},
	)
	require.NoError(t, err)

	committed, err := recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID: "state-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, committed.LogEntry)
	assert.Equal(t, "state_snapshot", committed.LogEntry.Kind)
	assert.Equal(t, new("author"), committed.LogEntry.Author)
	assert.Equal(t, new("reviewer"), committed.LogEntry.Committer)
	assert.Equal(t, "Current finding.", committed.LogEntry.Body)

	view, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "state-1"},
	)
	require.NoError(t, err)
	require.NotNil(t, view.Detail.State)
	require.NotNil(t, view.Detail.State.SnapshotLogEntryID)
	assert.Equal(t, committed.LogEntry.ID, *view.Detail.State.SnapshotLogEntryID)
	committedRevision := view.Detail.Issue.Revision

	duplicate, err := recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID: "state-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	assert.Nil(t, duplicate.LogEntry)
	assert.Equal(t, committedRevision, duplicate.Issue.Revision)

	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("author"),
		record.SetStateRequest{
			IssueID: "state-1", Text: "Current finding.",
		},
	)
	require.NoError(t, err)
	view, err = repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "state-1"},
	)
	require.NoError(t, err)
	require.NotNil(t, view.Detail.State)
	assert.Nil(t, view.Detail.State.SnapshotLogEntryID)

	duplicate, err = recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID: "state-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, duplicate.LogEntry)
	assert.NotEqual(t, committed.LogEntry.ID, duplicate.LogEntry.ID)
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "state-1"},
	)
	require.NoError(t, err)
	assert.Len(t, entries, 2)

	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("other-author"),
		record.SetStateRequest{
			IssueID: "state-1", Text: "Current finding.",
		},
	)
	require.NoError(t, err)
	view, err = repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "state-1"},
	)
	require.NoError(t, err)
	require.NotNil(t, view.Detail.State)
	assert.Nil(t, view.Detail.State.SnapshotLogEntryID)

	otherAuthor, err := recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID: "state-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, otherAuthor.LogEntry)
	assert.Equal(t, new("other-author"), otherAuthor.LogEntry.Author)
	assert.NotEqual(t, committed.LogEntry.ID, otherAuthor.LogEntry.ID)

	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("other-author"),
		record.SetStateRequest{
			IssueID: "state-1", Text: "Current finding.",
		},
	)
	require.NoError(t, err)
	duplicate, err = recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID: "state-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, duplicate.LogEntry)
	entries, err = repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "state-1"},
	)
	require.NoError(t, err)
	assert.Len(t, entries, 4)
}

func TestRepositoryCommitStatePreservesRepeatedChronology(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "chronology-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Repeated State chronology", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)

	var snapshots []issue.LogID
	for _, body := range []string{"A", "B", "A"} {
		_, err = recorder.SetState(
			t.Context(),
			issue.NewInvocation("author"),
			record.SetStateRequest{IssueID: "chronology-1", Text: body},
		)
		require.NoError(t, err)
		committed, err := recorder.CommitState(
			t.Context(),
			issue.NewInvocation("reviewer"),
			record.CommitStateRequest{
				IssueID:     "chronology-1",
				Disposition: record.CommitStateRetain,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, committed.LogEntry)
		snapshots = append(snapshots, committed.LogEntry.ID)
	}

	assert.NotEqual(t, snapshots[0], snapshots[2])
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "chronology-1"},
	)
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Equal(t, []string{"A", "B", "A"}, []string{
		entries[0].Body,
		entries[1].Body,
		entries[2].Body,
	})
}

func TestRepositoryDoesNotRelinkUnattributedStateSnapshot(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "imported-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Imported State", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)
	snapshotID := issue.LogID("log_11111111111111111111111111111111")
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issue_log_entries (
			id, board_id, issue_id, kind, body
		) VALUES (?, 'board-test', 'imported-1', 'state_snapshot', 'Imported State.');
		INSERT INTO issue_states (
			issue_id, board_id, body
		) VALUES ('imported-1', 'board-test', 'Imported State.');
	`, snapshotID)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	mutation, err := repository.beginMutation(t.Context())
	require.NoError(t, err)
	state, _, err := repository.readIssueState(
		t.Context(),
		mutation.change,
		issue.MustID("imported-1"),
	)
	require.NoError(t, err)
	state, err = repository.commitState(t.Context(), mutation, state)
	require.NoError(t, err)
	require.NoError(t, mutation.change.Done())

	recovery := state.RecoveryStateRecord()
	require.NotNil(t, recovery)
	assert.Nil(t, recovery.SnapshotLogEntryID)
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "imported-1"},
	)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestRepositoryRollsBackStateCommitWhenRevisionPublicationFails(
	t *testing.T,
) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "rollback-state-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Atomic State", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("author"),
		record.SetStateRequest{
			IssueID: "rollback-state-1", Text: "Must survive rollback.",
		},
	)
	require.NoError(t, err)

	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		CREATE TRIGGER reject_state_commit_revision
		BEFORE UPDATE OF revision ON boards
		BEGIN
			SELECT RAISE(ABORT, 'reject State commit revision');
		END
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	_, err = recorder.CommitState(
		t.Context(),
		issue.NewInvocation("reviewer"),
		record.CommitStateRequest{
			IssueID:     "rollback-state-1",
			Disposition: record.CommitStateClear,
		},
	)
	require.ErrorContains(t, err, "reject State commit revision")

	view, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "rollback-state-1"},
	)
	require.NoError(t, err)
	require.NotNil(t, view.Detail.State)
	assert.Equal(t, "Must survive rollback.", view.Detail.State.Body)
	assert.Nil(t, view.Detail.State.SnapshotLogEntryID)
	assert.Equal(t, new("Must survive rollback."), view.Detail.Issue.State)
	entries, err := repository.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "rollback-state-1"},
	)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestRepositoryOrdersHistoricalLogEntriesByLocalSequence(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "log-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Portable log", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issue_log_entries (
			local_sequence, id, board_id, issue_id, kind,
			author, committer, body, created_at
		) VALUES
			(
				5, 'cmt_ffffffffffffffffffffffffffffffff',
				'board-test', 'log-1', 'post',
				'captain', 'captain', 'First', 1700000000
			),
			(
				7, 'cmt_00000000000000000000000000000000',
				'board-test', 'log-1', 'post',
				'engineer', 'engineer', 'Second', 1700000000
			)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	entries, err := repository.ListLogEntries(t.Context(), issue.LogListRequest{IssueID: "log-1"})
	require.NoError(t, err)
	assert.Equal(t, []issue.LogEntry{
		{
			ID: "cmt_ffffffffffffffffffffffffffffffff", IssueID: "log-1",
			Kind: "post", Author: new("captain"), Committer: new("captain"),
			Body: "First", Created: new(int64(1_700_000_000)),
		},
		{
			ID: "cmt_00000000000000000000000000000000", IssueID: "log-1",
			Kind: "post", Author: new("engineer"), Committer: new("engineer"),
			Body: "Second", Created: new(int64(1_700_000_000)),
		},
	}, entries)

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "log-1"})
	require.NoError(t, err)
	latest, err := issue.NewLogID("cmt_00000000000000000000000000000000")
	require.NoError(t, err)
	assert.Equal(t, issue.LogSummary{Count: 2, LatestID: &latest}, view.Detail.LogSummary)

	snapshot, err := repository.ReadDumpSnapshot(t.Context())
	require.NoError(t, err)
	require.Len(t, snapshot.LogEntries, 2)
	assert.Equal(t, "cmt_ffffffffffffffffffffffffffffffff", snapshot.LogEntries[0].ID.String())
	assert.Equal(t, "cmt_00000000000000000000000000000000", snapshot.LogEntries[1].ID.String())
}

func TestRepositoryReadsReadyBlockedAndActionableCheckpoints(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "pool-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	requests := []planning.CreateIssueRequest{
		{Title: "Ready task", Type: "task", Priority: 1, Labels: []string{"ready"}},
		{Title: "Blocked task", Type: "task", Priority: 0, DependsOn: []string{"pool-1"}},
		{Title: "Blocked routine", Type: "routine", Priority: 0, DependsOn: []string{"pool-1"}},
		{Title: "Blocked checkpoint", Type: "checkpoint", Priority: 2, DependsOn: []string{"pool-1"}},
		{Title: "Actionable checkpoint", Type: "checkpoint", Priority: 2, Labels: []string{"gate"}},
		{Title: "Waiting task", Type: "task", Priority: 2},
		{Title: "Claimed blocked task", Type: "task", Priority: 2},
	}
	for _, request := range requests {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}
	_, err := executor.ClaimIssue(t.Context(), issue.NewInvocation("reviewer"), execution.ClaimIssueRequest{
		ID: "pool-6", Assignee: "reviewer",
	})
	require.NoError(t, err)
	waitingReason := "Awaiting external review"
	_, err = executor.ReleaseIssue(t.Context(), issue.NewInvocation("reviewer"), execution.ReleaseIssueRequest{
		ID: "pool-6", WaitingReason: &waitingReason,
	})
	require.NoError(t, err)
	_, err = executor.ClaimIssue(t.Context(), issue.NewInvocation("engineer"), execution.ClaimIssueRequest{
		ID: "pool-7", Assignee: "engineer",
	})
	require.NoError(t, err)
	_, err = planner.EditIssue(t.Context(), issue.NewInvocation("engineer"), planning.EditIssueRequest{
		ID: "pool-7", AddDependencies: []string{"pool-1"},
	})
	require.NoError(t, err)

	ready, err := repository.ListReadyIssues(t.Context(), issue.ListReadyRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-1"}, summaryIDs(ready))
	blocked, err := repository.ListBlockedIssues(t.Context(), issue.ListBlockedRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-2", "pool-4"}, summaryIDs(blocked))

	checkpoints, err := repository.ListActionableCheckpoints(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"pool-5"}, checkpointIDs(checkpoints))
	assert.Equal(t, []string{"gate"}, checkpoints[0].Labels)
}

func TestRepositoryReadsIssueStoryWithoutSiblingDescendants(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "story-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), planning.ApplyDocumentRequest{
		Version: 1, Mode: planning.ApplyModeCommit,
		Issues: []planning.ApplyIssue{
			{Alias: new("root"), Title: new("Root"), Type: new("workstream")},
			applyContainedIssue("aunt", "Aunt", "workstream", "root"),
			applyContainedIssue("parent", "Parent", "workstream", "root"),
			applyContainedIssue("cousin", "Cousin", "task", "aunt"),
			applyContainedIssue("sibling", "Sibling", "task", "parent"),
			applyContainedIssue("selected", "Selected", "workstream", "parent"),
			applyContainedIssue("child", "Child", "workstream", "selected"),
			applyContainedIssue("grandchild", "Grandchild", "task", "child"),
		},
	})
	require.NoError(t, err)

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "story-6"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"story-1", "story-2", "story-3", "story-5", "story-6", "story-7", "story-8",
	}, containmentIDs(view.Detail.Story.Containment))
	assert.NotContains(t, containmentIDs(view.Detail.Story.Containment), "story-4")
}

func TestRepositoryListActorsIgnoresUnattributedIssueCreation(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "actor-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation(""), planning.CreateIssueRequest{
		Title: "Unattributed work", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	actors, err := repository.ListActors(t.Context())
	require.NoError(t, err)
	assert.Empty(t, actors)
}

func summaryIDs(summaries []issue.Summary) []string {
	values := make([]string, len(summaries))
	for index, summary := range summaries {
		values[index] = summary.Issue.ID
	}
	return values
}

func checkpointIDs(checkpoints []issue.CheckpointView) []string {
	values := make([]string, len(checkpoints))
	for index, checkpoint := range checkpoints {
		values[index] = checkpoint.ID
	}
	return values
}

func containmentIDs(containment []issue.ContainmentNode) []string {
	values := make([]string, len(containment))
	for index, contained := range containment {
		values[index] = contained.ID
	}
	return values
}
