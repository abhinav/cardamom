package checkpointconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
	"go.uber.org/mock/gomock"
)

func TestServiceListActionableCheckpointsUsesGlobalDomainOrder(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One")
	boardTwo := testBoard(t, "board-2", projectOne.ID(), "Board Two")
	readerOne := NewMockBoardReader(gomock.NewController(t))
	readerOne.EXPECT().ListActionableCheckpoints(gomock.Any()).Return(
		[]issue.CheckpointView{{
			Issue: testIssue("checkpoint-1", "Gate one", "checkpoint", "open", "ready", 1, 20),
			Blocks: []issue.Reference{
				testIssueReference("blocked-1", "Blocked one", "task", "blocked", 2),
			},
			Labels: []string{"release"},
		}}, nil,
	)
	readerOne.EXPECT().ReadIssue(
		gomock.Any(), issue.ReadRequest{IssueID: "checkpoint-1", ContextDepth: new(0)},
	).Return(issue.View{Detail: issue.Detail{Issue: issue.Issue{
		ID: "checkpoint-1", Title: "Gate one", Type: "checkpoint",
		Lifecycle: "open", Status: "ready", Priority: 1,
		Created: 20, Updated: 20, Summary: new("Gate **one**"),
	}}}, nil)
	readerTwo := NewMockBoardReader(gomock.NewController(t))
	readerTwo.EXPECT().ListActionableCheckpoints(gomock.Any()).Return(
		[]issue.CheckpointView{{
			Issue: testIssue("checkpoint-2", "Gate two", "checkpoint", "open", "ready", 0, 30),
		}}, nil,
	)
	readerTwo.EXPECT().ReadIssue(
		gomock.Any(), issue.ReadRequest{IssueID: "checkpoint-2", ContextDepth: new(0)},
	).Return(issue.View{Detail: issue.Detail{
		Issue: testIssue("checkpoint-2", "Gate two", "checkpoint", "open", "ready", 0, 30),
	}}, nil)
	readers := NewMockBoardReaderFactory(gomock.NewController(t))
	readers.EXPECT().Reader(boardOne.ID()).Return(readerOne, nil)
	readers.EXPECT().Reader(boardTwo.ID()).Return(readerTwo, nil)
	client := newTestClient(t, testConfig{
		catalog:  &testCatalog{boards: []*board.State{boardOne, boardTwo}},
		locator:  &testIssueLocator{},
		readers:  readers,
		commands: NewMockBoardCommandFactory(gomock.NewController(t)),
	})

	response, err := client.ListActionableCheckpoints(
		t.Context(),
		connect.NewRequest(&privatev1.ListActionableCheckpointsRequest{
			Scope: &privatev1.BoardScope{
				Selection: &privatev1.BoardScope_AllBoards{AllBoards: &privatev1.AllBoards{}},
			},
		}),
	)
	require.NoError(t, err)
	require.Len(t, response.Msg.GetCheckpoints(), 2)
	assert.Equal(t, "checkpoint-2", response.Msg.GetCheckpoints()[0].GetCheckpoint().GetId())
	assert.Equal(t, "board-2", response.Msg.GetCheckpoints()[0].GetCheckpoint().GetBoardId())
	assert.Equal(t, "checkpoint-1", response.Msg.GetCheckpoints()[1].GetCheckpoint().GetId())
	assert.Contains(t, response.Msg.GetCheckpoints()[1].GetSummary().GetRenderedHtml(), "<strong>one</strong>")
	require.Len(t, response.Msg.GetCheckpoints()[1].GetBlockedIssues(), 1)
	assert.Equal(t, "blocked-1", response.Msg.GetCheckpoints()[1].GetBlockedIssues()[0].GetId())
}

func TestServiceResolveCheckpointReturnsCommittedDecisionSet(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One")
	reader := NewMockBoardReader(gomock.NewController(t))
	approvedDecision := issue.CheckpointDecisionView{
		Outcome: "approved", Reason: "approved", DecidedAt: 12, Revision: 3,
	}
	deniedDecision := issue.CheckpointDecisionView{
		Outcome: "denied", Reason: "denied", DecidedAt: 13, Revision: 4,
	}
	reader.EXPECT().ReadIssue(
		gomock.Any(), issue.ReadRequest{IssueID: "checkpoint"},
	).Return(issue.View{Detail: issue.Detail{
		Issue:  testIssue("checkpoint", "Gate", "checkpoint", "closed", "closed", 0, 10),
		Labels: []string{"approval"}, CheckpointDecision: &approvedDecision,
	}}, nil)
	reader.EXPECT().ReadIssue(
		gomock.Any(), issue.ReadRequest{IssueID: "checkpoint"},
	).Return(issue.View{Detail: issue.Detail{
		Issue:  testIssue("checkpoint", "Gate", "checkpoint", "cancelled", "cancelled", 0, 10),
		Labels: []string{"approval"}, CheckpointDecision: &deniedDecision,
	}}, nil)
	reader.EXPECT().ReadIssue(
		gomock.Any(), issue.ReadRequest{IssueID: "cancelled"},
	).Return(issue.View{Detail: issue.Detail{
		Issue: testIssue("cancelled", "Cancelled", "task", "cancelled", "cancelled", 1, 11),
	}}, nil)
	commands := NewMockBoardCommands(gomock.NewController(t))
	commands.EXPECT().ApproveCheckpoint(
		gomock.Any(), gomock.Any(), execution.CheckpointRequest{IssueID: "checkpoint", Reason: "approved"},
	).DoAndReturn(func(_ context.Context, invocation issue.Invocation, request execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
		assert.Equal(t, "approver", invocation.Actor())
		assert.Equal(t, "approved", request.Reason)
		checkpoint := testIssue("checkpoint", "Gate", "checkpoint", "closed", "closed", 0, 10)
		return execution.ResolveCheckpointResult{Decision: approvedDecision, Issue: &checkpoint}, nil
	})
	commands.EXPECT().DenyCheckpoint(
		gomock.Any(), gomock.Any(), execution.CheckpointRequest{IssueID: "checkpoint", Reason: "denied"},
	).DoAndReturn(func(_ context.Context, _ issue.Invocation, request execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
		assert.Equal(t, "denied", request.Reason)
		checkpoint := testIssue("checkpoint", "Gate", "checkpoint", "cancelled", "cancelled", 0, 10)
		return execution.ResolveCheckpointResult{
			Decision: deniedDecision,
			Cancelled: []issue.Issue{
				checkpoint,
				testIssue("cancelled", "Cancelled", "task", "cancelled", "cancelled", 1, 11),
			},
		}, nil
	})
	readers := NewMockBoardReaderFactory(gomock.NewController(t))
	readers.EXPECT().Reader(boardOne.ID()).Return(reader, nil).Times(3)
	commandFactory := NewMockBoardCommandFactory(gomock.NewController(t))
	commandFactory.EXPECT().Commands(boardOne.ID()).Return(commands, nil).Times(3)
	client := newTestClient(t, testConfig{
		catalog:  &testCatalog{boards: []*board.State{boardOne}},
		locator:  &testIssueLocator{values: map[string]board.ID{"checkpoint": boardOne.ID()}},
		readers:  readers,
		commands: commandFactory,
	})

	approved, err := client.ResolveCheckpoint(t.Context(), connect.NewRequest(&privatev1.ResolveCheckpointRequest{
		IssueId: "checkpoint", Outcome: privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_APPROVED,
		Reason: new("approved"), Context: mutationContext("approver"),
	}))
	require.NoError(t, err)
	assert.Equal(t, privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_APPROVED, approved.Msg.GetDecision().GetOutcome())
	assert.Equal(t, "approved", approved.Msg.GetDecision().GetReason().GetSource())
	assert.Equal(t, []string{"approval"}, approved.Msg.GetCheckpoint().GetLabels())
	assert.Empty(t, approved.Msg.GetCancelledDependents())

	denied, err := client.ResolveCheckpoint(t.Context(), connect.NewRequest(&privatev1.ResolveCheckpointRequest{
		IssueId: "checkpoint", Outcome: privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_DENIED,
		Reason: new("denied"), Context: mutationContext("approver"),
	}))
	require.NoError(t, err)
	assert.Equal(t, privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_DENIED, denied.Msg.GetDecision().GetOutcome())
	assert.Equal(t, "denied", denied.Msg.GetDecision().GetReason().GetSource())
	require.Len(t, denied.Msg.GetCancelledDependents(), 1)
	assert.Equal(t, "cancelled", denied.Msg.GetCancelledDependents()[0].GetId())

	_, err = client.ResolveCheckpoint(t.Context(), connect.NewRequest(&privatev1.ResolveCheckpointRequest{
		IssueId: "checkpoint",
	}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

type testConfig struct {
	catalog  *testCatalog
	locator  *testIssueLocator
	readers  BoardReaderFactory
	commands BoardCommandFactory
}

func newTestClient(t *testing.T, cfg testConfig) privatev1connect.CheckpointServiceClient {
	t.Helper()
	service := New(Config{
		Scope:   boardscope.New(cfg.catalog, cfg.locator),
		Readers: cfg.readers, Commands: cfg.commands,
		Views: issueview.New(markdown.New()),
	})
	_, handler := privatev1connect.NewCheckpointServiceHandler(service)
	httpClient := &http.Client{Transport: &testHandlerTransport{handler: handler}}
	return privatev1connect.NewCheckpointServiceClient(httpClient, "http://cardamom.test")
}

type testHandlerTransport struct {
	handler http.Handler
}

func (t *testHandlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

type testCatalog struct {
	boards []*board.State
}

func (c *testCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	for _, board := range c.boards {
		if board.ID() == id {
			return board, nil
		}
	}
	return nil, errkind.Errorf(errkind.NotFound, "board not found")
}

func (c *testCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return c.boards, nil
}

func (c *testCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c *testCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type testIssueLocator struct {
	values map[string]board.ID
}

func (l *testIssueLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l.values[issueID]
	if !ok {
		return "", errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return boardID, nil
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	value, err := project.Load(project.Snapshot{
		ID: project.ID(id), Name: name, Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

func testBoard(t *testing.T, id string, projectID project.ID, name string) *board.State {
	t.Helper()
	value, err := board.Load(board.Snapshot{
		ID: board.ID(id), ProjectID: projectID.String(),
		Name: name, Created: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

func testIssue(
	id, title, kind, lifecycle, status string,
	priority int,
	created int64,
) issue.Issue {
	return issue.Issue{
		ID: id, Title: title, Type: kind, Lifecycle: lifecycle, Status: status,
		Priority: priority, Created: created, Updated: created,
	}
}

func testIssueReference(
	id, title, kind, status string,
	priority int,
) issue.Reference {
	return issue.Reference{
		ID: id, Title: title, Type: kind, Status: status, Priority: priority,
	}
}

func mutationContext(actor string) *privatev1.MutationContext {
	return &privatev1.MutationContext{Actor: &actor}
}
