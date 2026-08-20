package aggregate

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

func (s *Server) getIssue(
	ctx context.Context,
	request *connect.Request[v1.GetIssueRequest],
) (*connect.Response[v1.GetIssueResponse], error) {
	if request == nil || request.Msg == nil || request.Msg.GetIssueId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("issue ID is required"))
	}
	route, err := s.issueRoute(
		ctx,
		request.Msg.GetIssueId(),
		request.Msg.GetSource(),
		request.Msg.GetBoardId(),
	)
	if err != nil {
		return nil, err
	}
	response, err := route.source.issues.GetIssue(ctx, connect.NewRequest(&v1.GetIssueRequest{
		IssueId: request.Msg.GetIssueId(), Presentation: aggregatePresentation(route.source),
	}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil || response.Msg.GetIssue() == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no issue"))
	}
	return connect.NewResponse(&v1.GetIssueResponse{
		Issue: qualifyDetail(route, response.Msg.GetIssue()),
	}), nil
}

func (s *Server) issueRoute(
	ctx context.Context,
	issueID string,
	sourceRef *v1.SourceRef,
	boardID string,
) (boardRoute, error) {
	if sourceRef != nil && boardID != "" {
		sourceValue, err := s.sourceForRef(sourceRef)
		if err != nil {
			return boardRoute{}, err
		}
		route, err := s.routeForBoard(boardID, sourceValue)
		if err != nil {
			return boardRoute{}, err
		}
		found, err := s.findIssueRoute(ctx, issueID, []*source{sourceValue})
		if err != nil {
			return boardRoute{}, err
		}
		if found.boardID != route.boardID {
			return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("issue not found"))
		}
		return route, nil
	}
	if sourceRef != nil {
		sourceValue, err := s.sourceForRef(sourceRef)
		if err != nil {
			return boardRoute{}, err
		}
		return s.findIssueRoute(ctx, issueID, []*source{sourceValue})
	}
	return s.findIssueRoute(ctx, issueID, s.sourcePointers())
}

func (s *Server) sourcePointers() []*source {
	result := make([]*source, 0, len(s.sources))
	for index := range s.sources {
		result = append(result, &s.sources[index])
	}
	return result
}

func (s *Server) findIssueRoute(
	ctx context.Context,
	issueID string,
	sources []*source,
) (boardRoute, error) {
	var found boardRoute
	foundCount := 0
	unavailable := false
	for _, source := range sources {
		response, err := source.issues.GetIssue(ctx, connect.NewRequest(&v1.GetIssueRequest{
			IssueId: issueID, Presentation: aggregatePresentation(source),
		}))
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound {
				continue
			}
			unavailable = true
			continue
		}
		if response == nil || response.Msg == nil || response.Msg.GetIssue() == nil {
			continue
		}
		boardID := response.Msg.GetIssue().GetIssue().GetBoardId()
		route, err := s.routeForBoard(boardID, source)
		if err != nil {
			continue
		}
		found, foundCount = route, foundCount+1
	}
	if foundCount == 1 {
		return found, nil
	}
	if foundCount > 1 {
		return boardRoute{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("source-qualified issue reference is required"))
	}
	if unavailable {
		return boardRoute{}, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	return boardRoute{}, connect.NewError(connect.CodeNotFound, errors.New("issue not found"))
}

func qualifyDetail(route boardRoute, value *v1.IssueDetail) *v1.IssueDetail {
	result := proto.Clone(value).(*v1.IssueDetail)
	if result.Issue != nil {
		result.Issue = qualifySummary(readTarget(route), result.Issue)
	}
	for _, ancestor := range result.GetContext().GetAncestors() {
		ancestor.Issue = qualifyRelated(route, ancestor.GetIssue())
	}
	for _, dependency := range result.GetContext().GetDependencyResults() {
		dependency.Issue = qualifyRelated(route, dependency.GetIssue())
	}
	for _, node := range result.GetContainment().GetNodes() {
		node.Issue = qualifyRelated(route, node.GetIssue())
	}
	for index, related := range result.GetPrerequisites() {
		result.Prerequisites[index] = qualifyRelated(route, related)
	}
	for index, related := range result.GetDependents() {
		result.Dependents[index] = qualifyRelated(route, related)
	}
	return result
}

func qualifyRelated(route boardRoute, value *v1.RelatedIssue) *v1.RelatedIssue {
	if value == nil {
		return nil
	}
	result := proto.Clone(value).(*v1.RelatedIssue)
	result.Source = sourceRefFromEntry(route.source)
	return result
}

type issueService struct {
	privatev1connect.UnimplementedIssueServiceHandler
	server *Server
}

func (s *issueService) ListIssues(
	ctx context.Context,
	request *connect.Request[v1.ListIssuesRequest],
) (*connect.Response[v1.ListIssuesResponse], error) {
	return s.server.listIssues(ctx, request)
}

func (s *issueService) GetIssue(
	ctx context.Context,
	request *connect.Request[v1.GetIssueRequest],
) (*connect.Response[v1.GetIssueResponse], error) {
	return s.server.getIssue(ctx, request)
}
