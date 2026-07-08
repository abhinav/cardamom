package executionconnect

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
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/markdown"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

func TestServiceMapsEligibilityAndClaimNext(t *testing.T) {
	t.Parallel()

	boardState := executionTestBoard(t)
	executor := &executionStub{
		ready: []issue.Summary{{Issue: executionTestIssue("an-ready")}},
	}
	service := New(Config{
		Scope: boardscope.New(
			executionCatalog{boardState},
			executionLocator{"an-ready": boardState.ID()},
		),
		Executors: executionFactory{boardState.ID(): executor},
		Views:     issueview.New(markdown.New()),
	})

	ready, err := service.ListReadyIssues(t.Context(), connect.NewRequest(&privatev1.ListReadyIssuesRequest{
		Scope: &privatev1.BoardScope{
			Selection: &privatev1.BoardScope_BoardId{BoardId: boardState.ID().String()},
		},
		Limit: 3,
	}))
	require.NoError(t, err)
	require.Len(t, ready.Msg.GetIssues(), 1)
	assert.Equal(t, "an-ready", ready.Msg.GetIssues()[0].GetId())
	assert.Equal(t, 3, executor.readyRequest.Limit)

	depth := uint32(0)
	claimed, err := service.ClaimNextIssue(t.Context(), connect.NewRequest(&privatev1.ClaimNextIssueRequest{
		BoardId:    boardState.ID().String(),
		Context:    &privatev1.MutationContext{Actor: new("worker")},
		AncestorId: new("an-parent"),
		LabelsAll:  []string{"area:protocol"}, LabelsAny: []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"},
		Watch:      true, ContextDepth: &depth,
	}))
	require.NoError(t, err)
	assert.Equal(t, "an-ready", claimed.Msg.GetIssue().GetIssue().GetId())
	assert.Equal(t, "worker", executor.invocation.Actor())
	assert.Equal(t, execution.ClaimNextRequest{
		UnderID: "an-parent", Assignee: "worker",
		LabelsAll: []string{"area:protocol"}, LabelsAny: []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"}, Watch: true, ContextDepth: new(0),
	}, executor.claimNext)
}

func TestServiceRejectsInvalidClaimNextLabels(t *testing.T) {
	t.Parallel()

	boardState := executionTestBoard(t)
	executor := new(executionStub)
	service := New(Config{
		Scope: boardscope.New(
			executionCatalog{boardState},
			executionLocator{},
		),
		Executors: executionFactory{boardState.ID(): executor},
		Views:     issueview.New(markdown.New()),
	})

	_, err := service.ClaimNextIssue(t.Context(), connect.NewRequest(&privatev1.ClaimNextIssueRequest{
		BoardId:    boardState.ID().String(),
		Context:    &privatev1.MutationContext{Actor: new("worker")},
		LabelsNone: []string{"-paused"},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Empty(t, executor.claimNext.Assignee)
}

type executionStub struct {
	ready        []issue.Summary
	readyRequest issue.ListReadyRequest
	invocation   issue.Invocation
	claimNext    execution.ClaimNextRequest
}

func (s *executionStub) ListReadyIssues(
	_ context.Context,
	request issue.ListReadyRequest,
) ([]issue.Summary, error) {
	s.readyRequest = request
	return s.ready, nil
}

func (*executionStub) ListBlockedIssues(
	context.Context,
	issue.ListBlockedRequest,
) ([]issue.Summary, error) {
	return nil, nil
}

func (*executionStub) ClaimIssue(
	context.Context,
	issue.Invocation,
	execution.ClaimIssueRequest,
) (execution.ClaimIssueResult, error) {
	return execution.ClaimIssueResult{}, nil
}

func (s *executionStub) ClaimNext(
	_ context.Context,
	invocation issue.Invocation,
	request execution.ClaimNextRequest,
) (execution.ClaimIssueResult, error) {
	s.invocation = invocation
	s.claimNext = request
	return execution.ClaimIssueResult{Issue: issue.View{
		Detail: issue.Detail{Issue: executionTestIssue("an-ready")},
	}}, nil
}

func (*executionStub) ReleaseIssue(
	context.Context,
	issue.Invocation,
	execution.ReleaseIssueRequest,
) (execution.ReleaseIssueResult, error) {
	return execution.ReleaseIssueResult{}, nil
}

func (*executionStub) CloseIssues(
	context.Context,
	issue.Invocation,
	execution.CloseIssuesRequest,
) (execution.CloseIssuesResult, error) {
	return execution.CloseIssuesResult{}, nil
}

func (*executionStub) CancelIssues(
	context.Context,
	issue.Invocation,
	execution.CancelIssuesRequest,
) (execution.CancelIssuesResult, error) {
	return execution.CancelIssuesResult{}, nil
}

func (*executionStub) ReopenIssues(
	context.Context,
	issue.Invocation,
	execution.ReopenIssuesRequest,
) (execution.ReopenIssuesResult, error) {
	return execution.ReopenIssuesResult{}, nil
}

type executionFactory map[board.ID]BoardExecutor

func (f executionFactory) Executor(boardID board.ID) (BoardExecutor, error) {
	executor, ok := f[boardID]
	if !ok {
		return nil, assert.AnError
	}
	return executor, nil
}

type executionCatalog struct{ board *board.State }

func (c executionCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return []*board.State{c.board}, nil
}

func (c executionCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	if c.board.ID() != id {
		return nil, errkind.Errorf(errkind.NotFound, "board not found")
	}
	return c.board, nil
}

func (c executionCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c executionCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type executionLocator map[string]board.ID

func (l executionLocator) BoardForIssue(_ context.Context, issueID string) (board.ID, error) {
	boardID, ok := l[issueID]
	if !ok {
		return "", errkind.Errorf(errkind.NotFound, "issue not found")
	}
	return boardID, nil
}

func executionTestBoard(t *testing.T) *board.State {
	t.Helper()
	state, err := board.Load(board.Snapshot{
		ID: "board-1", ProjectID: "project-1", Name: "Board",
		Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return state
}

func executionTestIssue(id string) issue.Issue {
	return issue.Issue{
		ID: id, Title: "Issue", Type: "task", Lifecycle: "open",
		Status: "ready", Priority: 2, Created: 1, Updated: 1,
	}
}
