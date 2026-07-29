// Package executionconnect exposes issue eligibility, custody, and lifecycle
// operations through Connect.
package executionconnect

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
	"go.abhg.dev/cardamom/internal/web/boardscope"
	"go.abhg.dev/cardamom/internal/web/issueview"
)

const defaultPoolLimit = 20

// BoardExecutor supplies the domain operations exposed by ExecutionService.
type BoardExecutor interface {
	// ListReadyIssues returns claimable executable work in domain order.
	ListReadyIssues(context.Context, issue.ListReadyRequest) ([]issue.Summary, error)

	// ListBlockedIssues returns unfinished work with unresolved prerequisites.
	ListBlockedIssues(context.Context, issue.ListBlockedRequest) ([]issue.Summary, error)

	// ClaimIssue acquires custody of one explicitly identified issue.
	ClaimIssue(context.Context, issue.Invocation, execution.ClaimIssueRequest) (execution.ClaimIssueResult, error)

	// ClaimNext selects and claims the next matching ready issue.
	ClaimNext(context.Context, issue.Invocation, execution.ClaimNextRequest) (execution.ClaimIssueResult, error)

	// ReleaseIssue relinquishes actor-owned custody.
	ReleaseIssue(context.Context, issue.Invocation, execution.ReleaseIssueRequest) (execution.ReleaseIssueResult, error)

	// CloseIssues completes issues in caller order.
	CloseIssues(context.Context, issue.Invocation, execution.CloseIssuesRequest) (execution.CloseIssuesResult, error)

	// CancelIssues abandons roots and their dependent closure.
	CancelIssues(context.Context, issue.Invocation, execution.CancelIssuesRequest) (execution.CancelIssuesResult, error)

	// ReopenIssues returns terminal issues to the open lifecycle.
	ReopenIssues(context.Context, issue.Invocation, execution.ReopenIssuesRequest) (execution.ReopenIssuesResult, error)
}

// BoardExecutorFactory opens execution operations for one resolved board.
type BoardExecutorFactory interface {
	// Executor returns execution operations constrained to boardID.
	Executor(board.ID) (BoardExecutor, error)
}

// Config supplies the collaborators required by ExecutionService.
type Config struct {
	// Scope resolves protocol scopes and issue ownership.
	Scope *boardscope.Resolver // required

	// Executors opens board-scoped execution operations.
	Executors BoardExecutorFactory // required

	// Views converts issue-domain records to generated messages.
	Views *issueview.Encoder // required
}

// Service translates ExecutionService requests to shared domain operations.
type Service struct {
	privatev1connect.UnimplementedExecutionServiceHandler
	scope     *boardscope.Resolver
	executors BoardExecutorFactory
	views     *issueview.Encoder
}

var _ privatev1connect.ExecutionServiceHandler = (*Service)(nil)

// New constructs an ExecutionService handler.
func New(cfg Config) *Service {
	must.NotBeNilf(cfg.Scope, "executionconnect: board scope resolver is required")
	must.NotBeNilf(cfg.Executors, "executionconnect: executor factory is required")
	must.NotBeNilf(cfg.Views, "executionconnect: issue view encoder is required")
	return &Service{scope: cfg.Scope, executors: cfg.Executors, views: cfg.Views}
}

// ListReadyIssues returns claimable executable work in global domain order.
func (s *Service) ListReadyIssues(
	ctx context.Context,
	request *connect.Request[privatev1.ListReadyIssuesRequest],
) (*connect.Response[privatev1.ListReadyIssuesResponse], error) {
	limit := poolLimit(request.Msg.GetLimit())
	values, err := s.listPool(ctx, request.Msg.GetScope(), limit, true)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListReadyIssuesResponse{Issues: values}), nil
}

// ListBlockedIssues returns unfinished work with unresolved prerequisites.
func (s *Service) ListBlockedIssues(
	ctx context.Context,
	request *connect.Request[privatev1.ListBlockedIssuesRequest],
) (*connect.Response[privatev1.ListBlockedIssuesResponse], error) {
	limit := poolLimit(request.Msg.GetLimit())
	values, err := s.listPool(ctx, request.Msg.GetScope(), limit, false)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListBlockedIssuesResponse{Issues: values}), nil
}

func (s *Service) listPool(
	ctx context.Context,
	scopeValue *privatev1.BoardScope,
	limit int,
	ready bool,
) ([]*privatev1.IssueSummary, error) {
	boards, err := s.scope.Boards(ctx, scopeValue)
	if err != nil {
		return nil, err
	}
	ordered := make([]issue.BoardIssueSummary, 0)
	for _, state := range boards {
		executor, err := s.executors.Executor(state.ID())
		if err != nil {
			return nil, fmt.Errorf("open board executor %q: %w", state.ID(), err)
		}
		var values []issue.Summary
		if ready {
			values, err = executor.ListReadyIssues(ctx, issue.ListReadyRequest{Limit: limit})
		} else {
			values, err = executor.ListBlockedIssues(ctx, issue.ListBlockedRequest{Limit: limit})
		}
		if err != nil {
			return nil, err
		}
		for _, value := range values {
			ordered = append(ordered, issue.BoardIssueSummary{
				BoardID: state.ID().String(), Summary: value,
			})
		}
	}
	ordered = issue.OrderSummaries(issue.ListRequest{}, ordered)
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	result := make([]*privatev1.IssueSummary, 0, len(ordered))
	for _, value := range ordered {
		converted, err := s.views.Summary(board.ID(value.BoardID), value.Summary)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

// ClaimIssue acquires custody of one explicitly identified issue.
func (s *Service) ClaimIssue(
	ctx context.Context,
	request *connect.Request[privatev1.ClaimIssueRequest],
) (*connect.Response[privatev1.ClaimIssueResponse], error) {
	boardID, executor, err := s.executorForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	actor := request.Msg.GetContext().GetActor()
	result, err := executor.ClaimIssue(
		ctx,
		issue.NewInvocation(actor),
		execution.ClaimIssueRequest{
			ID: request.Msg.GetIssueId(), Assignee: actor,
			ContextDepth: contextDepth(request.Msg.ContextDepth),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	return s.claimResponse(ctx, boardID, result)
}

// ClaimNextIssue selects and claims the next matching ready issue.
func (s *Service) ClaimNextIssue(
	ctx context.Context,
	request *connect.Request[privatev1.ClaimNextIssueRequest],
) (*connect.Response[privatev1.ClaimNextIssueResponse], error) {
	boardID, executor, err := s.executorForBoard(ctx, request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	actor := request.Msg.GetContext().GetActor()
	labelsAll, err := protocolLabels(request.Msg.GetLabelsAll())
	if err != nil {
		return nil, web.FromError(err)
	}
	labelsAny, err := protocolLabels(request.Msg.GetLabelsAny())
	if err != nil {
		return nil, web.FromError(err)
	}
	labelsNone, err := protocolLabels(request.Msg.GetLabelsNone())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := executor.ClaimNext(
		ctx,
		issue.NewInvocation(actor),
		execution.ClaimNextRequest{
			UnderID: request.Msg.GetAncestorId(), Assignee: actor,
			LabelsAll: labelsAll, LabelsAny: labelsAny, LabelsNone: labelsNone,
			Watch:        request.Msg.GetWatch(),
			ContextDepth: contextDepth(request.Msg.ContextDepth),
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.Detail(ctx, boardID, result.Issue)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ClaimNextIssueResponse{Issue: converted}), nil
}

// protocolLabels normalizes one unsigned protocol label group.
func protocolLabels(values []string) ([]string, error) {
	result := make([]string, len(values))
	for index, value := range values {
		label, err := issue.NewLabel(value)
		if err != nil {
			return nil, err
		}
		result[index] = label.String()
	}
	return result, nil
}

func (s *Service) claimResponse(
	ctx context.Context,
	boardID board.ID,
	result execution.ClaimIssueResult,
) (*connect.Response[privatev1.ClaimIssueResponse], error) {
	converted, err := s.views.Detail(ctx, boardID, result.Issue)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ClaimIssueResponse{Issue: converted}), nil
}

// ReleaseIssue relinquishes actor-owned custody while keeping the issue open.
func (s *Service) ReleaseIssue(
	ctx context.Context,
	request *connect.Request[privatev1.ReleaseIssueRequest],
) (*connect.Response[privatev1.ReleaseIssueResponse], error) {
	boardID, executor, err := s.executorForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := executor.ReleaseIssue(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		execution.ReleaseIssueRequest{
			ID: request.Msg.GetIssueId(), WaitingReason: request.Msg.WaitingReason,
		},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.Detail(ctx, boardID, issue.View{Detail: result.Issue})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ReleaseIssueResponse{Issue: converted}), nil
}

// CloseIssues completes issues in caller order.
func (s *Service) CloseIssues(
	ctx context.Context,
	request *connect.Request[privatev1.CloseIssuesRequest],
) (*connect.Response[privatev1.CloseIssuesResponse], error) {
	boardID, executor, err := s.executorForIssueSet(ctx, request.Msg.GetIssueIds())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := executor.CloseIssues(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		execution.CloseIssuesRequest{IDs: request.Msg.GetIssueIds()},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	issues, err := s.summaries(boardID, result.Issues)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CloseIssuesResponse{
		Issues:                     issues,
		ParentsWithoutOpenChildren: result.ParentsWithoutOpenChildren,
	}), nil
}

// CancelIssues abandons roots and their non-terminal dependent closure.
func (s *Service) CancelIssues(
	ctx context.Context,
	request *connect.Request[privatev1.CancelIssuesRequest],
) (*connect.Response[privatev1.CancelIssuesResponse], error) {
	boardID, executor, err := s.executorForIssueSet(ctx, request.Msg.GetRootIssueIds())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := executor.CancelIssues(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		execution.CancelIssuesRequest{Roots: request.Msg.GetRootIssueIds()},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	summaries := make([]issue.Summary, len(result.Issues))
	for index, value := range result.Issues {
		summaries[index] = issue.Summary{Issue: value}
	}
	issues, err := s.summaries(boardID, summaries)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CancelIssuesResponse{
		Issues: issues, Requested: uint32(result.Requested),
		Dependents:                 uint32(result.Dependents),
		ParentsWithoutOpenChildren: result.ParentsWithoutOpenChildren,
	}), nil
}

// ReopenIssues returns terminal issues to the open lifecycle.
func (s *Service) ReopenIssues(
	ctx context.Context,
	request *connect.Request[privatev1.ReopenIssuesRequest],
) (*connect.Response[privatev1.ReopenIssuesResponse], error) {
	boardID, executor, err := s.executorForIssueSet(ctx, request.Msg.GetIssueIds())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := executor.ReopenIssues(
		ctx,
		mutationInvocation(request.Msg.GetContext()),
		execution.ReopenIssuesRequest{IDs: request.Msg.GetIssueIds()},
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	response := &privatev1.ReopenIssuesResponse{
		Issues: make([]*privatev1.ReopenedIssue, 0, len(result.Issues)),
	}
	for _, value := range result.Issues {
		converted, err := s.views.Summary(boardID, value.Issue)
		if err != nil {
			return nil, web.FromError(err)
		}
		reopened := &privatev1.ReopenedIssue{Issue: converted}
		for _, prerequisite := range value.UnresolvedPrerequisites {
			status, err := protocolStatus(prerequisite.Status)
			if err != nil {
				return nil, web.FromError(err)
			}
			reopened.UnresolvedPrerequisites = append(
				reopened.UnresolvedPrerequisites,
				&privatev1.IssueStatusReference{IssueId: prerequisite.ID, Status: status},
			)
		}
		response.Issues = append(response.Issues, reopened)
	}
	return connect.NewResponse(response), nil
}

func (s *Service) executorForBoard(
	ctx context.Context,
	value string,
) (board.ID, BoardExecutor, error) {
	boardID, err := board.NewID(value)
	if err != nil {
		return "", nil, err
	}
	boards, err := s.scope.Boards(ctx, &privatev1.BoardScope{
		Selection: &privatev1.BoardScope_BoardId{BoardId: boardID.String()},
	})
	if err != nil {
		return "", nil, err
	}
	if len(boards) != 1 || boards[0].ID() != boardID {
		return "", nil, fmt.Errorf("resolve board %q", boardID)
	}
	executor, err := s.executors.Executor(boardID)
	if err != nil {
		return "", nil, fmt.Errorf("open board executor %q: %w", boardID, err)
	}
	return boardID, executor, nil
}

func (s *Service) executorForIssue(
	ctx context.Context,
	issueID string,
) (board.ID, BoardExecutor, error) {
	state, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return "", nil, err
	}
	executor, err := s.executors.Executor(state.ID())
	if err != nil {
		return "", nil, fmt.Errorf("open board executor %q: %w", state.ID(), err)
	}
	return state.ID(), executor, nil
}

func (s *Service) executorForIssueSet(
	ctx context.Context,
	issueIDs []string,
) (board.ID, BoardExecutor, error) {
	if len(issueIDs) == 0 {
		return "", nil, errkind.Errorf(errkind.InvalidInput, "invalid input: issue ID required")
	}
	return s.executorForIssue(ctx, issueIDs[0])
}

func (s *Service) summaries(
	boardID board.ID,
	values []issue.Summary,
) ([]*privatev1.IssueSummary, error) {
	result := make([]*privatev1.IssueSummary, 0, len(values))
	for _, value := range values {
		converted, err := s.views.Summary(boardID, value)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

func poolLimit(value uint32) int {
	if value == 0 {
		return defaultPoolLimit
	}
	return int(value)
}

func contextDepth(value *uint32) *int {
	if value == nil {
		return nil
	}
	depth := int(*value)
	return &depth
}

func protocolStatus(value string) (privatev1.IssueStatus, error) {
	switch value {
	case "ready":
		return privatev1.IssueStatus_ISSUE_STATUS_READY, nil
	case "blocked":
		return privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, nil
	case "in_progress":
		return privatev1.IssueStatus_ISSUE_STATUS_IN_PROGRESS, nil
	case "waiting":
		return privatev1.IssueStatus_ISSUE_STATUS_WAITING, nil
	case "closed":
		return privatev1.IssueStatus_ISSUE_STATUS_CLOSED, nil
	case "cancelled":
		return privatev1.IssueStatus_ISSUE_STATUS_CANCELLED, nil
	default:
		return 0, fmt.Errorf("convert issue status %q", value)
	}
}

func mutationInvocation(value *privatev1.MutationContext) issue.Invocation {
	return issue.NewInvocation(value.GetActor())
}
