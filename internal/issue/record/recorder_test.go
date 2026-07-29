package record

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestRecorderAttributesLogEntries(t *testing.T) {
	t.Parallel()

	changes := new(fakeChanges)
	recorder := NewRecorder(changes, new(fakeIssueReader))

	_, err := recorder.SetState(
		t.Context(),
		issue.NewInvocation("  alice  "),
		SetStateRequest{IssueID: "an-1", Text: "next"},
	)
	require.NoError(t, err)
	assert.Equal(t, issue.MustID("an-1"), changes.setState.IssueID)
	assert.Equal(t, issue.NewActor("alice"), changes.setState.Author)

	_, err = recorder.AddLogEntry(
		t.Context(),
		issue.NewInvocation("bob"),
		AddLogEntryRequest{IssueID: "an-1", Body: "done"},
	)
	require.NoError(t, err)
	assert.Equal(t, issue.NewActor("bob"), changes.addLogEntry.Author)
}

func TestNoOpAppendReadsCommittedProjection(t *testing.T) {
	t.Parallel()

	changes := &fakeChanges{appendOutcome: StateAppended{State: "current"}}
	reader := &fakeIssueReader{issue: issue.View{Detail: issue.Detail{
		Issue: issue.Issue{ID: "an-1", Revision: 17, State: new("current")},
	}}}
	recorder := NewRecorder(changes, reader)

	state, err := recorder.AppendState(
		t.Context(),
		issue.NewInvocation("operator"),
		SetStateRequest{IssueID: "an-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(17), state.Issue.Revision)
	assert.Equal(t, 1, reader.calls)
}

func TestRecorderExposesRecordReads(t *testing.T) {
	t.Parallel()

	reader := &fakeIssueReader{
		issue: issue.View{Detail: issue.Detail{
			Issue: issue.Issue{ID: "an-1", State: new("next")},
			State: &issue.RecoveryState{Body: "next"},
		}},
		logEntries: []issue.LogEntry{{
			ID: "cmt_00000000000000000000000000000001", IssueID: "an-1",
		}},
		result: issue.Result{IssueID: "an-1", Body: "done"},
	}
	recorder := NewRecorder(new(fakeChanges), reader)

	state, err := recorder.GetState(t.Context(), GetStateRequest{IssueID: "an-1"})
	require.NoError(t, err)
	assert.Equal(t, "an-1", state.IssueID)
	require.NotNil(t, state.State)
	assert.Equal(t, "next", state.State.Body)

	logEntries, err := recorder.ListLogEntries(
		t.Context(),
		issue.LogListRequest{IssueID: "an-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, reader.logEntries, logEntries)

	result, err := recorder.GetResult(t.Context(), GetResultRequest{IssueID: "an-1"})
	require.NoError(t, err)
	assert.Equal(t, reader.result, result)
}

func TestPolicyRejectsInvalidLogEntries(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-task", "")
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 5,
		Issue: state, OccurredAt: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)

	tests := []struct {
		name string
		give AddLogEntry
		want string
	}{
		{
			name: "MissingAuthor",
			give: AddLogEntry{IssueID: state.ID(), Body: "Finding."},
			want: "invalid input: log entry author required",
		},
		{
			name: "MissingBody",
			give: AddLogEntry{IssueID: state.ID(), Author: issue.NewActor("worker")},
			want: "invalid input: log entry body required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := policy.AddLogEntry(tt.give)
			assert.EqualError(t, err, tt.want)
		})
	}
}

func TestPolicyNormalizesAndValidatesResults(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-task", "")
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 5,
		Issue: state, OccurredAt: time.Unix(10, 0).UTC(),
	})
	require.NoError(t, err)

	t.Run("NormalizesBody", func(t *testing.T) {
		result, err := policy.SetResult(SetResult{
			IssueID: state.ID(), Body: "  Done.\n",
		})
		require.NoError(t, err)
		assert.Equal(t, "Done.", result.Body)
	})

	t.Run("RejectsEmptyNormalizedBody", func(t *testing.T) {
		_, err := policy.SetResult(SetResult{
			IssueID: state.ID(), Body: " \n\t",
		})
		assert.EqualError(t, err, "invalid input: result required")
	})
}

func TestPolicyAppendsToExistingState(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-state", "first")
	recovery := state.RecoveryStateRecord()
	recovery.NextAction = "Run the next diagnostic."
	state = state.WithRecoveryState(recovery, time.Unix(3, 0).UTC())
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 3,
		Issue: state, OccurredAt: time.Unix(4, 0).UTC(),
	})
	require.NoError(t, err)
	out, err := policy.AppendState(AppendState{
		IssueID: state.ID(),
		Author:  issue.NewActor("worker"),
		Text:    "second",
	})
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond", out.State)
	assert.Equal(
		t,
		"Run the next diagnostic.",
		out.Issue.RecoveryStateRecord().NextAction,
	)
}

func TestPolicySetState_emptyBodyClearsState(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-state", "temporary")
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 3,
		Issue: state, OccurredAt: time.Unix(4, 0).UTC(),
	})
	require.NoError(t, err)

	out, err := policy.SetState(SetState{IssueID: state.ID()})
	require.NoError(t, err)
	assert.Empty(t, out.State)
	assert.Nil(t, out.Issue.RecoveryStateRecord())
}

func TestPolicyCommitsStateWithSeparateAttribution(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-state", "current")
	recovery := state.RecoveryStateRecord()
	recovery.NextAction = "Notify the reviewer."
	state = state.WithRecoveryState(recovery, time.Unix(3, 0).UTC())
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 3,
		Issue: state, OccurredAt: time.Unix(4, 0).UTC(),
	})
	require.NoError(t, err)

	out, err := policy.CommitState(CommitState{
		IssueID: state.ID(), Committer: issue.NewActor("reviewer"),
		Disposition: CommitStateClear,
	})
	require.NoError(t, err)
	require.NotNil(t, out.LogEntry)
	assert.Equal(t, issue.LogEntryKindStateSnapshot, out.LogEntry.Kind)
	assert.Empty(t, out.LogEntry.Author)
	assert.Equal(t, issue.NewActor("reviewer"), out.LogEntry.Committer)
	assert.Equal(t, "current", out.LogEntry.Body)
	assert.Equal(t, "Notify the reviewer.", out.LogEntry.NextAction)
	assert.Empty(t, out.Issue.RecoveryState())
}

func TestPolicyDoesNotDuplicateLinkedState(t *testing.T) {
	t.Parallel()

	state := testIssueState(t, "an-state", "current")
	snapshotID := issue.LogID("log_11111111111111111111111111111111")
	state = state.WithRecoveryStateSnapshot(&snapshotID)
	policy, err := Load(Snapshot{
		BoardID: testBoardID(t), Revision: 3,
		Issue: state, OccurredAt: time.Unix(4, 0).UTC(),
	})
	require.NoError(t, err)

	out, err := policy.CommitState(CommitState{
		IssueID: state.ID(), Committer: issue.NewActor("reviewer"),
		Disposition: CommitStateRetain,
	})
	require.NoError(t, err)
	assert.False(t, out.Changed)
	assert.Nil(t, out.LogEntry)
}

func testBoardID(t *testing.T) board.ID {
	t.Helper()
	id, err := board.NewID("board")
	require.NoError(t, err)
	return id
}

func testIssueState(t *testing.T, id, recoveryState string) issue.State {
	t.Helper()
	var recovery *issue.RecoveryState
	if recoveryState != "" {
		recovery = &issue.RecoveryState{Body: recoveryState}
	}
	state, err := issue.Load(issue.Snapshot{
		ID:            issue.MustID(id),
		Title:         id,
		Kind:          issue.KindTask,
		Lifecycle:     issue.LifecycleOpen,
		Priority:      issue.PriorityNormal,
		Created:       time.Unix(1, 0).UTC(),
		Updated:       time.Unix(2, 0).UTC(),
		RecoveryState: recovery,
	})
	require.NoError(t, err)
	return state
}

type fakeIssueReader struct {
	issue      issue.View
	logEntries []issue.LogEntry
	result     issue.Result
	calls      int
}

func (f *fakeIssueReader) ReadIssue(
	_ context.Context,
	_ issue.ReadRequest,
) (issue.View, error) {
	f.calls++
	return f.issue, nil
}

func (f *fakeIssueReader) ListLogEntries(
	context.Context,
	issue.LogListRequest,
) ([]issue.LogEntry, error) {
	return f.logEntries, nil
}

func (f *fakeIssueReader) ReadResult(
	context.Context,
	issue.ResultRequest,
) (issue.Result, error) {
	return f.result, nil
}

type fakeChanges struct {
	setState      SetState
	addLogEntry   AddLogEntry
	appendOutcome StateAppended
}

func (f *fakeChanges) SetState(
	_ context.Context,
	command SetState,
) (StateSet, error) {
	f.setState = command
	return StateSet{}, nil
}

func (*fakeChanges) ClearState(
	context.Context,
	ClearState,
) (StateSet, error) {
	return StateSet{}, nil
}

func (f *fakeChanges) AppendState(
	context.Context,
	AppendState,
) (StateAppended, error) {
	return f.appendOutcome, nil
}

func (f *fakeChanges) AddLogEntry(
	_ context.Context,
	command AddLogEntry,
) (LogEntryAdded, error) {
	f.addLogEntry = command
	return LogEntryAdded{}, nil
}

func (*fakeChanges) CommitState(
	context.Context,
	CommitState,
) (StateCommitted, error) {
	return StateCommitted{}, nil
}

func (*fakeChanges) SetResult(
	context.Context,
	SetResult,
) (ResultSet, error) {
	return ResultSet{}, nil
}
