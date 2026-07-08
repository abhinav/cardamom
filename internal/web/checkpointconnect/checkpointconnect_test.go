package checkpointconnect

import (
	"context"
	"errors"
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
)

func TestServiceListActionableCheckpointsUsesGlobalDomainOrder(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One")
	boardTwo := testBoard(t, "board-2", projectOne.ID(), "Board Two")
	readerOne := &testBoardReader{
		checkpoints: []issue.CheckpointView{{
			Issue: testIssue("checkpoint-1", "Gate one", "checkpoint", "open", "ready", 1, 20),
			Blocks: []issue.Reference{
				testIssueReference("blocked-1", "Blocked one", "task", "blocked", 2),
			},
			Labels: []string{"release"},
		}},
		views: map[string]issue.View{
			"checkpoint-1": {Detail: issue.Detail{Issue: issue.Issue{
				ID: "checkpoint-1", Title: "Gate one", Type: "checkpoint",
				Lifecycle: "open", Status: "ready", Priority: 1,
				Created: 20, Updated: 20, Summary: new("Gate **one**"),
			}}},
		},
	}
	readerTwo := &testBoardReader{
		checkpoints: []issue.CheckpointView{{
			Issue: testIssue("checkpoint-2", "Gate two", "checkpoint", "open", "ready", 0, 30),
		}},
		views: map[string]issue.View{
			"checkpoint-2": {Detail: issue.Detail{
				Issue: testIssue("checkpoint-2", "Gate two", "checkpoint", "open", "ready", 0, 30),
			}},
		},
	}
	client := newTestClient(t, testConfig{
		catalog: &testCatalog{boards: []*board.State{boardOne, boardTwo}},
		locator: &testIssueLocator{},
		readers: &testBoardReaders{values: map[board.ID]*testBoardReader{
			boardOne.ID(): readerOne,
			boardTwo.ID(): readerTwo,
		}},
		commands: &testBoardCommandFactory{values: map[board.ID]*testBoardCommands{}},
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
	assert.Equal(t, []string{"checkpoint-1"}, readerOne.readIssueIDs)
}

func TestServiceResolveCheckpointReturnsCommittedDecisionSet(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One")
	reader := &testBoardReader{views: map[string]issue.View{
		"checkpoint": {Detail: issue.Detail{
			Issue:  testIssue("checkpoint", "Gate", "checkpoint", "closed", "closed", 0, 10),
			Labels: []string{"approval"},
		}},
		"cancelled": {Detail: issue.Detail{
			Issue: testIssue("cancelled", "Cancelled", "task", "cancelled", "cancelled", 1, 11),
		}},
	}}
	commands := &testBoardCommands{
		approve: func(_ context.Context, invocation issue.Invocation, request execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
			assert.Equal(t, "approver", invocation.Actor())
			assert.Equal(t, "approved", request.Reason)
			checkpoint := testIssue("checkpoint", "Gate", "checkpoint", "closed", "closed", 0, 10)
			decision := issue.CheckpointDecisionView{
				Outcome: "approved", Reason: "approved", DecidedAt: 12, Revision: 3,
			}
			reader.views["checkpoint"] = issue.View{Detail: issue.Detail{
				Issue: checkpoint, Labels: []string{"approval"}, CheckpointDecision: &decision,
			}}
			return execution.ResolveCheckpointResult{Decision: decision, Issue: &checkpoint}, nil
		},
		deny: func(_ context.Context, _ issue.Invocation, request execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
			assert.Equal(t, "denied", request.Reason)
			decision := issue.CheckpointDecisionView{
				Outcome: "denied", Reason: "denied", DecidedAt: 13, Revision: 4,
			}
			checkpoint := testIssue("checkpoint", "Gate", "checkpoint", "cancelled", "cancelled", 0, 10)
			reader.views["checkpoint"] = issue.View{Detail: issue.Detail{
				Issue: checkpoint, Labels: []string{"approval"}, CheckpointDecision: &decision,
			}}
			return execution.ResolveCheckpointResult{
				Decision: decision,
				Cancelled: []issue.Issue{
					checkpoint,
					testIssue("cancelled", "Cancelled", "task", "cancelled", "cancelled", 1, 11),
				},
			}, nil
		},
	}
	client := newTestClient(t, testConfig{
		catalog: &testCatalog{boards: []*board.State{boardOne}},
		locator: &testIssueLocator{values: map[string]board.ID{"checkpoint": boardOne.ID()}},
		readers: &testBoardReaders{values: map[board.ID]*testBoardReader{boardOne.ID(): reader}},
		commands: &testBoardCommandFactory{values: map[board.ID]*testBoardCommands{
			boardOne.ID(): commands,
		}},
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
	readers  *testBoardReaders
	commands *testBoardCommandFactory
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

type testBoardReaders struct {
	values map[board.ID]*testBoardReader
}

func (f *testBoardReaders) Reader(boardID board.ID) (BoardReader, error) {
	reader, ok := f.values[boardID]
	if !ok || reader == nil {
		return nil, errors.New("test board reader not configured")
	}
	return reader, nil
}

type testBoardReader struct {
	views        map[string]issue.View
	checkpoints  []issue.CheckpointView
	readIssueIDs []string
}

func (r *testBoardReader) ReadIssue(
	_ context.Context,
	request issue.ReadRequest,
) (issue.View, error) {
	r.readIssueIDs = append(r.readIssueIDs, request.IssueID)
	view, ok := r.views[request.IssueID]
	if !ok {
		return issue.View{}, errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return view, nil
}

func (r *testBoardReader) ListActionableCheckpoints(context.Context) ([]issue.CheckpointView, error) {
	return r.checkpoints, nil
}

type testBoardCommandFactory struct {
	values map[board.ID]*testBoardCommands
}

func (f *testBoardCommandFactory) Commands(boardID board.ID) (BoardCommands, error) {
	commands, ok := f.values[boardID]
	if !ok || commands == nil {
		return nil, errors.New("test board commands not configured")
	}
	return commands, nil
}

type testBoardCommands struct {
	approve func(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)
	deny    func(context.Context, issue.Invocation, execution.CheckpointRequest) (execution.ResolveCheckpointResult, error)
}

func (c *testBoardCommands) ApproveCheckpoint(
	ctx context.Context,
	invocation issue.Invocation,
	request execution.CheckpointRequest,
) (execution.ResolveCheckpointResult, error) {
	return c.approve(ctx, invocation, request)
}

func (c *testBoardCommands) DenyCheckpoint(
	ctx context.Context,
	invocation issue.Invocation,
	request execution.CheckpointRequest,
) (execution.ResolveCheckpointResult, error) {
	return c.deny(ctx, invocation, request)
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
