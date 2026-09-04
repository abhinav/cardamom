package record

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestRecorderAttributesLogEntries(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().SetState(gomock.Any(), SetState{
		IssueID: issue.MustID("an-1"), Author: issue.NewActor("alice"), Text: "next",
	}).Return(StateSet{}, nil)
	changes.EXPECT().AddLogEntry(gomock.Any(), AddLogEntry{
		IssueID: issue.MustID("an-1"), Author: issue.NewActor("bob"), Body: "done",
	}).Return(LogEntryAdded{}, nil)
	reader := NewMockReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), gomock.Any()).Return(issue.View{}, nil)
	recorder := NewRecorder(changes, reader)

	_, err := recorder.SetState(
		t.Context(),
		issue.NewInvocation("  alice  "),
		SetStateRequest{IssueID: "an-1", Text: "next"},
	)
	require.NoError(t, err)

	_, err = recorder.AddLogEntry(
		t.Context(),
		issue.NewInvocation("bob"),
		AddLogEntryRequest{IssueID: "an-1", Body: "done"},
	)
	require.NoError(t, err)
}

func TestNoOpAppendReadsCommittedProjection(t *testing.T) {
	t.Parallel()

	changes := NewMockChanges(gomock.NewController(t))
	changes.EXPECT().AppendState(gomock.Any(), AppendState{
		IssueID: issue.MustID("an-1"), Author: issue.NewActor("operator"),
	}).Return(StateAppended{State: "current"}, nil)
	view := issue.View{Detail: issue.Detail{
		Issue: issue.Issue{ID: "an-1", Revision: 17, State: new("current")},
	}}
	reader := NewMockReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), gomock.Any()).Return(view, nil)
	recorder := NewRecorder(changes, reader)

	state, err := recorder.AppendState(
		t.Context(),
		issue.NewInvocation("operator"),
		SetStateRequest{IssueID: "an-1"},
	)
	require.NoError(t, err)
	assert.Equal(t, int64(17), state.Issue.Revision)
}

func TestRecorderExposesRecordReads(t *testing.T) {
	t.Parallel()

	issueView := issue.View{Detail: issue.Detail{
		Issue: issue.Issue{ID: "an-1", State: new("next")},
		State: &issue.RecoveryState{Body: "next"},
	}}
	expectedLogEntries := []issue.LogEntry{{
		ID: "cmt_00000000000000000000000000000001", IssueID: "an-1",
	}}
	expectedLogEntry := expectedLogEntries[0]
	expectedResult := issue.Result{IssueID: "an-1", Body: "done"}
	reader := NewMockReader(gomock.NewController(t))
	reader.EXPECT().ReadIssue(gomock.Any(), issue.ReadRequest{IssueID: "an-1"}).Return(issueView, nil)
	reader.EXPECT().ListLogEntries(gomock.Any(), issue.LogListRequest{IssueID: "an-1"}).Return(expectedLogEntries, nil)
	reader.EXPECT().ReadLogEntry(gomock.Any(), GetLogEntryRequest{
		LogID: "cmt_00000000000000000000000000000001",
	}).Return(expectedLogEntry, nil)
	reader.EXPECT().ReadResult(gomock.Any(), issue.ResultRequest{IssueID: "an-1"}).Return(expectedResult, nil)
	recorder := NewRecorder(NewMockChanges(gomock.NewController(t)), reader)

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
	assert.Equal(t, []issue.LogEntry{{
		ID: "cmt_00000000000000000000000000000001", IssueID: "an-1",
	}}, logEntries)
	logEntry, err := recorder.GetLogEntry(t.Context(), GetLogEntryRequest{
		LogID: "cmt_00000000000000000000000000000001",
	})
	require.NoError(t, err)
	assert.Equal(t, expectedLogEntry, logEntry)

	result, err := recorder.GetResult(t.Context(), GetResultRequest{IssueID: "an-1"})
	require.NoError(t, err)
	assert.Equal(t, issue.Result{IssueID: "an-1", Body: "done"}, result)
}

func TestRecorderRejectsInvalidLogReadIdentity(t *testing.T) {
	t.Parallel()

	recorder := NewRecorder(
		NewMockChanges(gomock.NewController(t)),
		NewMockReader(gomock.NewController(t)),
	)

	_, err := recorder.GetLogEntry(
		t.Context(),
		GetLogEntryRequest{LogID: "an-1"},
	)
	assert.ErrorContains(t, err, `invalid log ID "an-1"`)
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
