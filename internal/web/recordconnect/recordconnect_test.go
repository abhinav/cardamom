package recordconnect

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
	"go.uber.org/mock/gomock"
)

func TestServiceKeepsLogEntriesStatesAndResultsDistinct(t *testing.T) {
	t.Parallel()

	boardState := recordTestBoard(t)
	records := NewMockBoardRecords(gomock.NewController(t))
	records.EXPECT().GetState(
		gomock.Any(), record.GetStateRequest{IssueID: "an-1"},
	).Return(record.GetStateResult{
		IssueID: "an-1",
		State: &issue.RecoveryState{
			Body: "current", NextAction: "next",
		},
	}, nil)
	records.EXPECT().AddLogEntry(
		gomock.Any(),
		issue.NewInvocation("engineer"),
		record.AddLogEntryRequest{IssueID: "an-1", Body: "diagnostic"},
	).Return(record.AddLogEntryResult{LogEntry: issue.LogEntry{
		ID:      "cmt_77777777777777777777777777777777",
		IssueID: "an-1", Kind: "post",
		Author: new("engineer"), Committer: new("engineer"),
		Body: "diagnostic", Created: new(int64(1)),
	}}, nil)
	records.EXPECT().SetResult(
		gomock.Any(),
		issue.NewInvocation("engineer"),
		record.SetResultRequest{IssueID: "an-1", Body: "done"},
	).Return(record.SetResultResult{IssueID: "an-1", Body: "done"}, nil)
	records.EXPECT().GetResult(
		gomock.Any(), record.GetResultRequest{IssueID: "an-1"},
	).Return(issue.Result{IssueID: "an-1", Title: "Task", Body: "done"}, nil)
	factory := NewMockBoardRecordFactory(gomock.NewController(t))
	factory.EXPECT().Records(boardState.ID()).Return(records, nil).Times(3)
	service := New(Config{
		Scope: boardscope.New(
			recordCatalog{boardState},
			recordLocator{"an-1": boardState.ID()},
		),
		Records: factory,
		Views:   issueview.New(markdown.New()),
	})

	state, err := service.GetState(t.Context(), connect.NewRequest(&privatev1.GetStateRequest{
		IssueId: "an-1",
	}))
	require.NoError(t, err)
	assert.Equal(t, "current", state.Msg.GetState().GetBody().GetSource())
	assert.Equal(t, "next", state.Msg.GetState().GetNextAction().GetSource())

	logEntry, err := service.AddLogEntry(t.Context(), connect.NewRequest(&privatev1.AddLogEntryRequest{
		IssueId: "an-1", BodySource: "diagnostic",
		Context: &privatev1.MutationContext{Actor: new("engineer")},
	}))
	require.NoError(t, err)
	assert.Equal(t, "cmt_77777777777777777777777777777777", logEntry.Msg.GetLogEntry().GetId())
	assert.Equal(t, "engineer", logEntry.Msg.GetLogEntry().GetPost().GetActor())
	assert.Equal(t, "diagnostic", logEntry.Msg.GetLogEntry().GetPost().GetBody().GetSource())

	result, err := service.SetResult(t.Context(), connect.NewRequest(&privatev1.SetResultRequest{
		IssueId: "an-1", BodySource: "done",
		Context: &privatev1.MutationContext{Actor: new("engineer")},
	}))
	require.NoError(t, err)
	assert.Equal(t, "done", result.Msg.GetResult().GetBody().GetSource())
}

func TestServiceRendersLogEntryListInRequestedOrder(t *testing.T) {
	boardState := recordTestBoard(t)
	logEntries := []issue.LogEntry{
		{
			ID:      "cmt_11111111111111111111111111111111",
			IssueID: "an-1", Kind: "post",
			Author: new("one"), Committer: new("one"),
			Body: "First", Created: new(int64(1)),
		},
		{
			ID:      "cmt_22222222222222222222222222222222",
			IssueID: "an-1", Kind: "post",
			Author: new("two"), Committer: new("two"),
			Body: "Second", Created: new(int64(2)),
		},
	}
	records := NewMockBoardRecords(gomock.NewController(t))
	records.EXPECT().ListLogEntries(
		gomock.Any(),
		issue.LogListRequest{IssueID: "an-1", Reverse: true},
	).Return(logEntries, nil)
	factory := NewMockBoardRecordFactory(gomock.NewController(t))
	factory.EXPECT().Records(boardState.ID()).Return(records, nil)
	service := New(Config{
		Scope: boardscope.New(
			recordCatalog{boardState},
			recordLocator{"an-1": boardState.ID()},
		),
		Records: factory,
		Views:   issueview.New(markdown.New()),
	})

	response, err := service.ListLogEntries(
		t.Context(),
		connect.NewRequest(&privatev1.ListLogEntriesRequest{
			IssueId:   "an-1",
			Direction: privatev1.SortDirection_SORT_DIRECTION_DESCENDING,
		}),
	)
	require.NoError(t, err)

	require.Len(t, response.Msg.GetLogEntries(), 2)
	assert.Equal(t, "First", response.Msg.GetLogEntries()[0].GetPost().GetBody().GetSource())
	assert.Equal(t, "Second", response.Msg.GetLogEntries()[1].GetPost().GetBody().GetSource())
}

func TestServiceMapsStateCommitDispositionsAndTypedResponses(t *testing.T) {
	t.Parallel()

	boardState := recordTestBoard(t)
	updatedAt := time.Unix(2, 0).UTC()
	snapshotID := issue.LogID("log_11111111111111111111111111111111")
	stateSnapshot := func(body string) *issue.LogEntry {
		return &issue.LogEntry{
			ID: snapshotID, IssueID: "an-1", Kind: "state_snapshot",
			Author: new("author"), Committer: new("reviewer"),
			Body: body, NextAction: new("Preserved action."),
			Created: new(int64(2)),
		}
	}
	issueSummary := issue.Issue{
		ID: "an-1", Title: "Task", Type: "task",
		Lifecycle: "open", Status: "ready", Priority: 2,
		Created: 1, Updated: 2, Revision: 3,
	}
	commitResults := []record.CommitStateResult{
		{
			Issue: issueSummary,
			State: &issue.RecoveryState{
				Body: "Retained State.", NextAction: "Preserved action.",
				Author:             "author",
				UpdatedAt:          &updatedAt,
				SnapshotLogEntryID: &snapshotID,
			},
			LogEntry: stateSnapshot("Retained State."),
		},
		{
			Issue: issueSummary,
			State: &issue.RecoveryState{
				Body:       "Replacement State.",
				NextAction: "Replacement action.",
				Author:     "reviewer", UpdatedAt: &updatedAt,
			},
			LogEntry: stateSnapshot("Previous State."),
		},
		{
			Issue:    issueSummary,
			LogEntry: stateSnapshot("Cleared State."),
		},
	}
	records := NewMockBoardRecords(gomock.NewController(t))
	gomock.InOrder(
		records.EXPECT().CommitState(
			gomock.Any(), issue.NewInvocation("reviewer"),
			record.CommitStateRequest{
				IssueID: "an-1", Disposition: record.CommitStateRetain,
			},
		).Return(commitResults[0], nil),
		records.EXPECT().CommitState(
			gomock.Any(), issue.NewInvocation("reviewer"),
			record.CommitStateRequest{
				IssueID: "an-1", Disposition: record.CommitStateReplace,
				Replacement: record.StateReplacement{
					Body: "Replacement State.", NextAction: "Replacement action.",
				},
			},
		).Return(commitResults[1], nil),
		records.EXPECT().CommitState(
			gomock.Any(), issue.NewInvocation("reviewer"),
			record.CommitStateRequest{
				IssueID: "an-1", Disposition: record.CommitStateClear,
			},
		).Return(commitResults[2], nil),
	)
	factory := NewMockBoardRecordFactory(gomock.NewController(t))
	factory.EXPECT().Records(boardState.ID()).Return(records, nil).Times(3)
	service := New(Config{
		Scope: boardscope.New(
			recordCatalog{boardState},
			recordLocator{"an-1": boardState.ID()},
		),
		Records: factory,
		Views:   issueview.New(markdown.New()),
	})

	retained, err := service.CommitState(
		t.Context(),
		connect.NewRequest(&privatev1.CommitStateRequest{
			IssueId: "an-1",
			Context: &privatev1.MutationContext{Actor: new("reviewer")},
			Disposition: &privatev1.CommitStateRequest_Retain{
				Retain: &privatev1.RetainCommittedState{},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, retained.Msg.GetState())
	assert.Equal(t, "Retained State.", retained.Msg.GetState().GetBody().GetSource())
	assert.Equal(
		t,
		"Preserved action.",
		retained.Msg.GetState().GetNextAction().GetSource(),
	)
	assert.Equal(t, "author", retained.Msg.GetState().GetActor())
	assert.Equal(t, snapshotID.String(), retained.Msg.GetState().GetSnapshotLogEntryId())
	assert.Equal(t, int64(2), retained.Msg.GetState().GetUpdatedAt().GetSeconds())
	require.NotNil(t, retained.Msg.GetLogEntry())
	assert.IsType(
		t,
		&privatev1.LogEntry_StateSnapshot{},
		retained.Msg.GetLogEntry().GetPayload(),
	)
	retainedSnapshot := retained.Msg.GetLogEntry().GetStateSnapshot()
	require.NotNil(t, retainedSnapshot)
	assert.Equal(t, "author", retainedSnapshot.GetAuthor())
	assert.Equal(t, "reviewer", retainedSnapshot.GetCommitter())
	assert.Equal(
		t,
		"Preserved action.",
		retainedSnapshot.GetNextAction().GetSource(),
	)
	assert.Equal(t, int64(2), retainedSnapshot.GetCreatedAt().GetSeconds())

	replaced, err := service.CommitState(
		t.Context(),
		connect.NewRequest(&privatev1.CommitStateRequest{
			IssueId: "an-1",
			Context: &privatev1.MutationContext{Actor: new("reviewer")},
			Disposition: &privatev1.CommitStateRequest_Replace{
				Replace: &privatev1.ReplaceCommittedState{
					BodySource:       "Replacement State.",
					NextActionSource: new("Replacement action."),
				},
			},
		}),
	)
	require.NoError(t, err)
	require.NotNil(t, replaced.Msg.GetState())
	assert.Equal(
		t,
		"Replacement State.",
		replaced.Msg.GetState().GetBody().GetSource(),
	)
	assert.Equal(
		t,
		"Replacement action.",
		replaced.Msg.GetState().GetNextAction().GetSource(),
	)

	cleared, err := service.CommitState(
		t.Context(),
		connect.NewRequest(&privatev1.CommitStateRequest{
			IssueId: "an-1",
			Context: &privatev1.MutationContext{Actor: new("reviewer")},
			Disposition: &privatev1.CommitStateRequest_Clear{
				Clear: &privatev1.ClearCommittedState{},
			},
		}),
	)
	require.NoError(t, err)
	assert.Nil(t, cleared.Msg.GetState())
	require.NotNil(t, cleared.Msg.GetLogEntry())
	assert.IsType(
		t,
		&privatev1.LogEntry_StateSnapshot{},
		cleared.Msg.GetLogEntry().GetPayload(),
	)
	assert.Equal(
		t,
		"Cleared State.",
		cleared.Msg.GetLogEntry().GetStateSnapshot().GetBody().GetSource(),
	)
}

type recordCatalog struct{ board *board.State }

func (c recordCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return []*board.State{c.board}, nil
}

func (c recordCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	if c.board.ID() != id {
		return nil, errkind.Errorf(errkind.NotFound, "board not found")
	}
	return c.board, nil
}

func (c recordCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c recordCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type recordLocator map[string]board.ID

func (l recordLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l[issueID]
	if !ok {
		return "", errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return boardID, nil
}

func recordTestBoard(t *testing.T) *board.State {
	t.Helper()
	state, err := board.Load(board.Snapshot{
		ID: "board-1", ProjectID: "project-1", Name: "Board",
		Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}
