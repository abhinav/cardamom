package aggregate

import (
	"context"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

// conflictService routes single-board reads without permitting aggregate writes.
// The shared read-only interceptor rejects ResolveConflict before dispatch.
type conflictService struct {
	privatev1connect.UnimplementedConflictServiceHandler
	server *Server
}

func (s *conflictService) ListConflicts(ctx context.Context, request *connect.Request[v1.ListConflictsRequest]) (*connect.Response[v1.ListConflictsResponse], error) {
	scope := request.Msg.GetScope()
	route, err := s.server.routeForBoard(scope.GetBoardId(), scope.GetSource())
	if err != nil {
		return nil, err
	}
	input := proto.Clone(request.Msg).(*v1.ListConflictsRequest)
	input.Scope = &v1.ConflictScope{BoardId: route.boardID}
	result, err := route.source.conflicts.ListConflicts(ctx, connect.NewRequest(input))
	if err != nil {
		return nil, err
	}
	out := proto.Clone(result.Msg).(*v1.ListConflictsResponse)
	for _, summary := range out.Conflicts {
		qualifyConflictSummary(route, summary)
	}
	return connect.NewResponse(out), nil
}

func (s *conflictService) GetConflict(ctx context.Context, request *connect.Request[v1.GetConflictRequest]) (*connect.Response[v1.GetConflictResponse], error) {
	scope := request.Msg.GetScope()
	route, err := s.server.routeForBoard(scope.GetBoardId(), scope.GetSource())
	if err != nil {
		return nil, err
	}
	input := proto.Clone(request.Msg).(*v1.GetConflictRequest)
	input.Scope = &v1.ConflictScope{BoardId: route.boardID}
	result, err := route.source.conflicts.GetConflict(ctx, connect.NewRequest(input))
	if err != nil {
		return nil, err
	}
	out := proto.Clone(result.Msg).(*v1.GetConflictResponse)
	qualifyConflictSummary(route, out.GetConflict().GetSummary())
	qualifyConflictIssues(route, out.GetConflict().GetIssues())
	return connect.NewResponse(out), nil
}

func (s *conflictService) GetConflictAlternative(ctx context.Context, request *connect.Request[v1.GetConflictAlternativeRequest]) (*connect.Response[v1.GetConflictAlternativeResponse], error) {
	scope := request.Msg.GetScope()
	route, err := s.server.routeForBoard(scope.GetBoardId(), scope.GetSource())
	if err != nil {
		return nil, err
	}
	input := proto.Clone(request.Msg).(*v1.GetConflictAlternativeRequest)
	input.Scope = &v1.ConflictScope{BoardId: route.boardID}
	result, err := route.source.conflicts.GetConflictAlternative(ctx, connect.NewRequest(input))
	if err != nil {
		return nil, err
	}
	out := proto.Clone(result.Msg).(*v1.GetConflictAlternativeResponse)
	qualifyConflictIssues(route, out.GetValue().GetIssues())
	return connect.NewResponse(out), nil
}

func qualifyConflictSummary(route boardRoute, value *v1.ConflictSummary) {
	for index, ref := range value.GetIssues() {
		value.Issues[index] = qualifyRelated(route, ref)
	}
}

func qualifyConflictIssues(route boardRoute, values []*v1.ConflictIssue) {
	for _, value := range values {
		value.Issue = qualifyRelated(route, value.Issue)
	}
}
