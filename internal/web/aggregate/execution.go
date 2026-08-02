package aggregate

import (
	"context"
	"errors"
	"sort"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

// executionReadMode selects one of the read-only eligibility collections.
type executionReadMode uint8

const (
	readyExecutionRead executionReadMode = iota
	blockedExecutionRead
)

func (s *Server) listExecution(
	ctx context.Context,
	scope *v1.BoardScope,
	limit uint32,
	mode executionReadMode,
) ([]*v1.IssueSummary, *v1.AggregateStatus, error) {
	targets, err := s.targets(scope)
	if err != nil {
		return nil, nil, err
	}
	problems := make(map[string]string)
	successes := 0
	var values []*v1.IssueSummary
	for _, target := range targets {
		if mode == blockedExecutionRead {
			response, err := target.source.execution.ListBlockedIssues(ctx, connect.NewRequest(
				&v1.ListBlockedIssuesRequest{Scope: targetScope(target), Limit: limit},
			))
			if err != nil || response == nil || response.Msg == nil {
				problems[target.source.config.Alias] = "source unavailable"
				continue
			}
			successes++
			for _, value := range response.Msg.GetIssues() {
				values = append(values, qualifySummary(target, value))
			}
			continue
		}
		response, err := target.source.execution.ListReadyIssues(ctx, connect.NewRequest(
			&v1.ListReadyIssuesRequest{Scope: targetScope(target), Limit: limit},
		))
		if err != nil || response == nil || response.Msg == nil {
			problems[target.source.config.Alias] = "source unavailable"
			continue
		}
		successes++
		for _, value := range response.Msg.GetIssues() {
			values = append(values, qualifySummary(target, value))
		}
	}
	sort.SliceStable(values, func(left, right int) bool {
		return compareIssueSummary(values[left], values[right]) < 0
	})
	if pageSize := aggregatePageSize(limit); len(values) > pageSize {
		values = values[:pageSize]
	}
	if len(targets) > 0 && successes == 0 {
		return nil, nil, connect.NewError(connect.CodeUnavailable, errors.New("no aggregate sources are available"))
	}
	return values, aggregateStatus(problems), nil
}

type executionService struct {
	privatev1connect.UnimplementedExecutionServiceHandler
	server *Server
}

func (s *executionService) ListReadyIssues(
	ctx context.Context,
	request *connect.Request[v1.ListReadyIssuesRequest],
) (*connect.Response[v1.ListReadyIssuesResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("execution request is required"))
	}
	issues, status, err := s.server.listExecution(
		ctx, request.Msg.GetScope(), request.Msg.GetLimit(), readyExecutionRead,
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ListReadyIssuesResponse{
		Issues: issues, AggregateStatus: status,
	}), nil
}

func (s *executionService) ListBlockedIssues(
	ctx context.Context,
	request *connect.Request[v1.ListBlockedIssuesRequest],
) (*connect.Response[v1.ListBlockedIssuesResponse], error) {
	if request == nil || request.Msg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("execution request is required"))
	}
	issues, status, err := s.server.listExecution(
		ctx, request.Msg.GetScope(), request.Msg.GetLimit(), blockedExecutionRead,
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ListBlockedIssuesResponse{
		Issues: issues, AggregateStatus: status,
	}), nil
}
