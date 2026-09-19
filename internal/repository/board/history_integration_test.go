package board

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryMutationObservesEveryRetainedTip(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "frontier-",
		IDStrategy: "sequential",
	})
	baseline := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, baseline, 1)
	localWriter := baseline[0].marker.Header().Writer
	otherWriter := retainIndependentHistoryTip(t, repository)
	planner := planning.NewPlanner(repository, repository, nil)

	_, err := planner.CreateIssue(
		t.Context(), issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)

	mutations := readBoardHistory(t, repository.store, repository.boardID)
	var localSecond *boardhistory.Marker
	for _, mutation := range mutations {
		header := mutation.marker.Header()
		if header.Writer == localWriter && header.Sequence == 2 {
			localSecond = mutation.marker
			break
		}
	}
	require.NotNil(t, localSecond)
	frontier := localSecond.Header().Frontier
	require.Len(t, frontier, 2)
	assert.ElementsMatch(t, []domainboard.WriterUID{
		localWriter, otherWriter,
	}, []domainboard.WriterUID{frontier[0].Writer, frontier[1].Writer})
}

func TestRepositoryRetainsOneMutationPerProductOperation(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "history-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	externalKey := "source:first"

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "First", Type: "task", Priority: 2,
			Labels: []string{"alpha"}, Key: &externalKey,
		},
	)
	require.NoError(t, err)
	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 2)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueOrigin,
		boardhistory.EventExternalKey,
		boardhistory.EventIssueLabel,
	}, historyEventKinds(mutations[1].events))
	assertHistoryChain(t, mutations)

	title := "Renamed"
	kind := "workstream"
	priority := 1
	summary := "Stable summary"
	details := "Expanded details"
	labels := []string{"beta"}
	_, err = planner.EditIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.EditIssueRequest{
			ID: "history-1", Title: &title, Type: &kind, Priority: &priority,
			Summary: &summary, SummarySet: true,
			Details: &details, DetailsSet: true,
			Labels: &labels,
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 3)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueTitleSet,
		boardhistory.EventIssueKindSet,
		boardhistory.EventIssuePrioritySet,
		boardhistory.EventIssueSummarySet,
		boardhistory.EventIssueDetailsSet,
		boardhistory.EventIssueLabel,
		boardhistory.EventIssueLabel,
	}, historyEventKinds(mutations[2].events))
	assertHistoryChain(t, mutations)

	_, err = planner.EditIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.EditIssueRequest{
			ID: "history-1", SummarySet: true, DetailsSet: true,
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 4)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueSummarySet,
		boardhistory.EventIssueDetailsSet,
	}, historyEventKinds(mutations[3].events))

	for _, request := range []planning.CreateIssueRequest{
		{Title: "Prerequisite", Type: "task", Priority: 2},
		{
			Title: "Dependent", Type: "task", Priority: 2,
			DependsOn: []string{"history-2"},
		},
	} {
		_, err = planner.CreateIssue(
			t.Context(), issue.NewInvocation("captain"), request,
		)
		require.NoError(t, err)
	}
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueOrigin,
		boardhistory.EventIssueDependency,
	}, historyEventKinds(mutations[len(mutations)-1].events))

	document := applyRequest(
		applyIssue("child", "Child", "task"),
		applyIssue("parent", "Parent", "workstream"),
	)
	document.Mode = planning.ApplyModeCommit
	document.Issues[0].Parent = planning.ApplyParentChange{
		Kind: planning.ParentReplace, Reference: aliasReference("parent"),
	}
	_, err = planner.ApplyDocument(
		t.Context(), issue.NewInvocation("captain"), document,
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 7)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueOrigin,
		boardhistory.EventIssueOrigin,
		boardhistory.EventIssueContainment,
	}, historyEventKinds(mutations[6].events))
	assertHistoryChain(t, mutations)
}

func TestRepositoryCanonicalizesWhitespaceOnlyIssueMarkdown(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "whitespace-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	whitespace := " \t\n"

	created, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Task", Type: "task", Priority: 2,
			Summary: whitespace, Details: whitespace, Key: &whitespace,
		},
	)
	require.NoError(t, err)
	assert.Nil(t, created.Issue.Issue.Summary)
	assert.Nil(t, created.Issue.Issue.Details)

	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 2)
	require.Len(t, mutations[1].events, 2)
	origin, ok := mutations[1].events[0].Payload.(*boardhistory.IssueOrigin)
	require.True(t, ok)
	assert.Nil(t, origin.Summary)
	assert.Nil(t, origin.Details)
	key, ok := mutations[1].events[1].Payload.(*boardhistory.ExternalKey)
	require.True(t, ok)
	assert.Equal(t, whitespace, key.Key)

	beforeEdit, err := repository.store.CanonicalRevision(t.Context())
	require.NoError(t, err)
	edited, err := planner.EditIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.EditIssueRequest{
			ID: "whitespace-1", Summary: &whitespace, SummarySet: true,
			Details: &whitespace, DetailsSet: true,
		},
	)
	require.NoError(t, err)
	assert.Nil(t, edited.Issue.Issue.Summary)
	assert.Nil(t, edited.Issue.Issue.Details)
	afterEdit, err := repository.store.CanonicalRevision(t.Context())
	require.NoError(t, err)
	assert.Equal(t, beforeEdit+1, afterEdit)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, 3)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueSummarySet,
		boardhistory.EventIssueDetailsSet,
	}, historyEventKinds(mutations[2].events))
	summary, ok := mutations[2].events[0].Payload.(*boardhistory.IssueSummarySet)
	require.True(t, ok)
	assert.Nil(t, summary.Summary)
	details, ok := mutations[2].events[1].Payload.(*boardhistory.IssueDetailsSet)
	require.True(t, ok)
	assert.Nil(t, details.Details)
}

func TestRepositoryRetainsSharedReleaseButNotClaim(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "claim-history-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	beforeClaim := readBoardHistory(t, repository.store, repository.boardID)

	_, err = executor.ClaimIssue(
		t.Context(),
		issue.NewInvocation("worker"),
		execution.ClaimIssueRequest{ID: "claim-history-1", Assignee: "worker"},
	)
	require.NoError(t, err)
	assert.Len(t, readBoardHistory(t, repository.store, repository.boardID), len(beforeClaim))

	reason := "Awaiting review"
	_, err = executor.ReleaseIssue(
		t.Context(),
		issue.NewInvocation("worker"),
		execution.ReleaseIssueRequest{
			ID: "claim-history-1", WaitingReason: &reason,
		},
	)
	require.NoError(t, err)
	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, len(beforeClaim)+1)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueWaitingSet,
	}, historyEventKinds(mutations[len(mutations)-1].events))

	_, err = executor.ClaimIssue(
		t.Context(),
		issue.NewInvocation("next-worker"),
		execution.ClaimIssueRequest{
			ID: "claim-history-1", Assignee: "next-worker",
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, len(beforeClaim)+2)
	waitingClear, ok := mutations[len(mutations)-1].events[0].Payload.(*boardhistory.IssueWaitingSet)
	require.True(t, ok)
	assert.Nil(t, waitingClear.Waiting)

	beforeLocalRelease := len(mutations)
	_, err = executor.ReleaseIssue(
		t.Context(), issue.NewInvocation("next-worker"),
		execution.ReleaseIssueRequest{ID: "claim-history-1"},
	)
	require.NoError(t, err)
	assert.Len(
		t,
		readBoardHistory(t, repository.store, repository.boardID),
		beforeLocalRelease,
	)
}

func TestRepositoryRetainsStateResultAndPinSemantics(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "record-history-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)

	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("worker"),
		record.SetStateRequest{IssueID: "record-history-1", Text: "Working"},
	)
	require.NoError(t, err)
	_, err = recorder.ClearState(
		t.Context(),
		issue.NewInvocation("worker"),
		record.ClearStateRequest{IssueID: "record-history-1"},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("worker"),
		record.SetResultRequest{IssueID: "record-history-1", Body: "First result"},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("worker"),
		record.SetResultRequest{IssueID: "record-history-1", Body: "Replacement result"},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("worker"),
		record.SetResultRequest{IssueID: "record-history-1", Body: "Replacement result"},
	)
	require.NoError(t, err)

	limit, err := domainboard.NewPinLimit(2)
	require.NoError(t, err)
	_, err = repository.PinIssue(t.Context(), issue.ID("record-history-1"), limit)
	require.NoError(t, err)
	beforeNoop := readBoardHistory(t, repository.store, repository.boardID)
	_, err = repository.PinIssue(t.Context(), issue.ID("record-history-1"), limit)
	require.NoError(t, err)
	assert.Len(t, readBoardHistory(t, repository.store, repository.boardID), len(beforeNoop))
	_, err = repository.UnpinIssue(t.Context(), issue.ID("record-history-1"))
	require.NoError(t, err)

	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.GreaterOrEqual(t, len(mutations), 9)
	stateClear := mutations[3].events
	require.Len(t, stateClear, 1)
	clearedState, ok := stateClear[0].Payload.(*boardhistory.IssueState)
	require.True(t, ok)
	assert.Nil(t, clearedState.State)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueResult,
	}, historyEventKinds(mutations[4].events))
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueResult,
	}, historyEventKinds(mutations[5].events))
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueResult,
	}, historyEventKinds(mutations[6].events))
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventBoardPin,
	}, historyEventKinds(mutations[len(mutations)-2].events))
	unpin, ok := mutations[len(mutations)-1].events[0].Payload.(*boardhistory.BoardPin)
	require.True(t, ok)
	assert.False(t, unpin.Present)
}

func TestRepositoryRetainsActualPinPosition(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "pin-history-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	for _, title := range []string{"First", "Middle", "Last"} {
		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.CreateIssueRequest{Title: title, Type: "task", Priority: 2},
		)
		require.NoError(t, err)
	}
	limit, err := domainboard.NewPinLimit(4)
	require.NoError(t, err)
	for _, id := range []issue.ID{"pin-history-1", "pin-history-2", "pin-history-3"} {
		_, err := repository.PinIssue(t.Context(), id, limit)
		require.NoError(t, err)
	}
	_, err = repository.UnpinIssue(t.Context(), "pin-history-2")
	require.NoError(t, err)
	_, err = repository.PinIssue(t.Context(), "pin-history-2", limit)
	require.NoError(t, err)

	mutations := readBoardHistory(t, repository.store, repository.boardID)
	repinned, ok := mutations[len(mutations)-1].events[0].Payload.(*boardhistory.BoardPin)
	require.True(t, ok)
	assert.Equal(t, int64(4), repinned.Position)
}

func TestRepositoryRetainsLifecycleAndCheckpointBatches(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "life-hist-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Closable", Type: "task", Priority: 2},
		{Title: "Cancellation root", Type: "task", Priority: 2},
		{
			Title: "Cancellation dependent", Type: "task", Priority: 2,
			DependsOn: []string{"life-hist-2"},
		},
		{Title: "Checkpoint", Type: "checkpoint", Priority: 1},
		{Title: "Denied checkpoint", Type: "checkpoint", Priority: 1},
		{
			Title: "After denied checkpoint", Type: "task", Priority: 2,
			DependsOn: []string{"life-hist-5"},
		},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}

	before := len(readBoardHistory(t, repository.store, repository.boardID))
	_, err := executor.CloseIssues(
		t.Context(), issue.NewInvocation("captain"),
		execution.CloseIssuesRequest{IDs: []string{"life-hist-1"}},
	)
	require.NoError(t, err)
	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	assert.Contains(t, historyEventKinds(mutations[len(mutations)-1].events), boardhistory.EventIssueLifecycleSet)

	before = len(mutations)
	_, err = executor.CancelIssues(
		t.Context(), issue.NewInvocation("captain"),
		execution.CancelIssuesRequest{Roots: []string{"life-hist-2"}},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	assert.Equal(t, 2, countHistoryEvents(
		mutations[len(mutations)-1].events,
		boardhistory.EventIssueLifecycleSet,
	))

	before = len(mutations)
	_, err = executor.ApproveCheckpoint(
		t.Context(), issue.NewInvocation("reviewer"),
		execution.CheckpointRequest{
			IssueID: "life-hist-4", Reason: "Ready",
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	checkpointKinds := historyEventKinds(mutations[len(mutations)-1].events)
	assert.Contains(t, checkpointKinds, boardhistory.EventIssueCheckpoint)
	assert.Contains(t, checkpointKinds, boardhistory.EventIssueLifecycleSet)

	before = len(mutations)
	_, err = executor.DenyCheckpoint(
		t.Context(), issue.NewInvocation("reviewer"),
		execution.CheckpointRequest{
			IssueID: "life-hist-5", Reason: "Not ready",
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	denialEvents := mutations[len(mutations)-1].events
	assert.Equal(t, 1, countHistoryEvents(
		denialEvents,
		boardhistory.EventIssueCheckpoint,
	))
	assert.Equal(t, 2, countHistoryEvents(
		denialEvents,
		boardhistory.EventIssueLifecycleSet,
	))
}

func TestRepositoryRetainsLogStateCommitAndReopen(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "record-op-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	executor := execution.NewExecutor(repository, repository)
	_, err := planner.CreateIssue(
		t.Context(), issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Task", Type: "task", Priority: 2},
	)
	require.NoError(t, err)

	before := len(readBoardHistory(t, repository.store, repository.boardID))
	_, err = recorder.AddLogEntry(
		t.Context(), issue.NewInvocation("author"),
		record.AddLogEntryRequest{IssueID: "record-op-1", Body: "Observation"},
	)
	require.NoError(t, err)
	mutations := readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventIssueLog,
	}, historyEventKinds(mutations[len(mutations)-1].events))

	stateAt := time.Unix(1_700_000_001, 0).UTC()
	repository.clock = fixedClock{now: stateAt}
	_, err = recorder.SetState(
		t.Context(), issue.NewInvocation("author"),
		record.SetStateRequest{IssueID: "record-op-1", Text: "Recovery position"},
	)
	require.NoError(t, err)
	before = len(readBoardHistory(t, repository.store, repository.boardID))
	commitAt := time.Unix(1_700_000_002, 0).UTC()
	repository.clock = fixedClock{now: commitAt}
	_, err = recorder.CommitState(
		t.Context(), issue.NewInvocation("committer"),
		record.CommitStateRequest{
			IssueID: "record-op-1", Disposition: record.CommitStateRetain,
		},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	commitKinds := historyEventKinds(mutations[len(mutations)-1].events)
	assert.Contains(t, commitKinds, boardhistory.EventIssueLog)
	assert.Contains(t, commitKinds, boardhistory.EventIssueState)
	for _, event := range mutations[len(mutations)-1].events {
		state, ok := event.Payload.(*boardhistory.IssueState)
		if !ok {
			continue
		}
		assert.Equal(t, commitAt, event.OccurredAt)
		require.NotNil(t, state.State)
		require.NotNil(t, state.State.UpdatedAt)
		assert.Equal(t, stateAt, *state.State.UpdatedAt)
	}

	_, err = executor.CloseIssues(
		t.Context(), issue.NewInvocation("captain"),
		execution.CloseIssuesRequest{IDs: []string{"record-op-1"}},
	)
	require.NoError(t, err)
	before = len(readBoardHistory(t, repository.store, repository.boardID))
	_, err = executor.ReopenIssues(
		t.Context(), issue.NewInvocation("captain"),
		execution.ReopenIssuesRequest{IDs: []string{"record-op-1"}},
	)
	require.NoError(t, err)
	mutations = readBoardHistory(t, repository.store, repository.boardID)
	require.Len(t, mutations, before+1)
	assert.Contains(
		t,
		historyEventKinds(mutations[len(mutations)-1].events),
		boardhistory.EventIssueLifecycleSet,
	)
}

func TestRepositoryRollsBackProjectionWhenHistoryAppendFails(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "rollback-",
		IDStrategy: "sequential",
	})
	installHistoryFailure(t, repository.store)
	planner := planning.NewPlanner(repository, repository, nil)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Must roll back", Type: "task", Priority: 2},
	)
	require.ErrorContains(t, err, "history failure")
	assert.Len(t, readBoardHistory(t, repository.store, repository.boardID), 1)
	issues, err := repository.ListIssues(t.Context(), issue.ListRequest{})
	require.NoError(t, err)
	assert.Empty(t, issues)
}

type retainedHistoryMutation struct {
	marker *boardhistory.Marker
	events []boardhistory.Event
}

func readBoardHistory(
	t *testing.T,
	persistence *store.Store,
	boardID domainboard.ID,
) []retainedHistoryMutation {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	rows, err := view.QueryContext(t.Context(), `
SELECT marker_bytes
FROM board_mutation_commits
WHERE board_id = ?
ORDER BY writer_uid, sequence
`, boardID.String())
	require.NoError(t, err)
	var markerBytes [][]byte
	for rows.Next() {
		var encoded []byte
		require.NoError(t, rows.Scan(&encoded))
		markerBytes = append(markerBytes, append([]byte(nil), encoded...))
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	mutations := make([]retainedHistoryMutation, 0, len(markerBytes))
	for _, encoded := range markerBytes {
		marker, err := boardhistory.DecodeMarker(encoded)
		require.NoError(t, err)
		var events []boardhistory.Event
		for retained, err := range boardhistory.MutationEvents(
			marker,
			func(index int64) ([]byte, error) {
				var part []byte
				header := marker.Header()
				err := view.QueryRowContext(t.Context(), `
SELECT bytes
FROM board_mutation_parts
WHERE board_id = ? AND writer_uid = ? AND sequence = ? AND part_index = ?
`, boardID.String(), header.Writer.Bytes(), header.Sequence, index).Scan(&part)
				return part, err
			},
		) {
			require.NoError(t, err)
			events = append(events, retained.Event)
		}
		mutations = append(mutations, retainedHistoryMutation{
			marker: marker, events: events,
		})
	}
	return mutations
}

func assertHistoryChain(t *testing.T, mutations []retainedHistoryMutation) {
	t.Helper()
	for index := 1; index < len(mutations); index++ {
		previous := mutations[index-1].marker
		current := mutations[index].marker
		header := current.Header()
		assert.Equal(t, uint64(index+1), header.Sequence)
		assert.Equal(t, previous.Hash(), header.Previous)
		require.Len(t, header.Frontier, 1)
		assert.Equal(t, previous.Header().Writer, header.Frontier[0].Writer)
		assert.Equal(t, previous.Header().Sequence, header.Frontier[0].Sequence)
		assert.Equal(t, previous.Hash(), header.Frontier[0].Hash)
	}
}

func historyEventKinds(events []boardhistory.Event) []boardhistory.EventKind {
	kinds := make([]boardhistory.EventKind, len(events))
	for index, event := range events {
		kinds[index] = event.Kind()
	}
	return kinds
}

func countHistoryEvents(events []boardhistory.Event, kind boardhistory.EventKind) int {
	var count int
	for _, event := range events {
		if event.Kind() == kind {
			count++
		}
	}
	return count
}

func installHistoryFailure(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
CREATE TRIGGER fail_board_history_append
BEFORE INSERT ON board_mutation_parts
WHEN NEW.sequence > 1
BEGIN
    SELECT RAISE(ABORT, 'history failure');
END
`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func retainIndependentHistoryTip(
	t *testing.T,
	repository *Repository,
) domainboard.WriterUID {
	t.Helper()
	writer, err := domainboard.ParseWriterUID(bytes.Repeat([]byte{0xfe}, 16))
	require.NoError(t, err)
	at := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	var parts []boardhistory.Part
	marker, err := boardhistory.EncodeMutation(
		boardhistory.Header{
			Board:  repository.boardID,
			Writer: writer, Sequence: 1,
		},
		func(yield func(boardhistory.Event, error) bool) {
			yield(boardhistory.Event{
				OccurredAt: at,
				Payload:    &boardhistory.BoardNameSet{Name: "Observed replica"},
			}, nil)
		},
		func(part boardhistory.Part) error {
			parts = append(parts, part)
			return nil
		},
	)
	require.NoError(t, err)
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	for _, part := range parts {
		_, err = change.ExecContext(t.Context(), `
INSERT INTO board_mutation_parts (
    board_id, writer_uid, sequence, part_index, bytes
) VALUES (?, ?, 1, ?, ?)
`, repository.boardID.String(), writer.Bytes(), part.Index, part.Bytes)
		require.NoError(t, err)
	}
	_, err = change.ExecContext(t.Context(), `
INSERT INTO board_mutation_commits (
    board_id, writer_uid, sequence, previous_hash, marker_hash,
    marker_bytes, part_count, event_count, payload_hash
) VALUES (?, ?, 1, ?, ?, ?, ?, ?, ?)
`, repository.boardID.String(), writer.Bytes(), marker.Header().Previous.Bytes(),
		marker.Hash().Bytes(), marker.Bytes(), marker.PartCount(),
		marker.EventCount(), marker.PayloadHash().Bytes())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO board_mutation_tips (board_id, writer_uid, sequence, marker_hash)
VALUES (?, ?, 1, ?)
`, repository.boardID.String(), writer.Bytes(), marker.Hash().Bytes())
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	return writer
}
