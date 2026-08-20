package aggregate

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
)

func (s *Server) listBoardPins(
	ctx context.Context,
	request *connect.Request[v1.ListBoardPinsRequest],
) (*connect.Response[v1.ListBoardPinsResponse], error) {
	if request == nil || request.Msg == nil || request.Msg.GetBoardId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("board ID is required"))
	}
	var sourceValue *source
	if sourceRef := request.Msg.GetSource(); sourceRef != nil {
		var err error
		sourceValue, err = s.sourceForRef(sourceRef)
		if err != nil {
			return nil, err
		}
	}
	route, err := s.routeForBoard(request.Msg.GetBoardId(), sourceValue)
	if err != nil {
		return nil, err
	}
	response, err := route.source.issues.ListBoardPins(ctx, connect.NewRequest(
		&v1.ListBoardPinsRequest{BoardId: route.boardID},
	))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no board pins"))
	}
	issues := make([]*v1.RelatedIssue, 0, len(response.Msg.GetIssues()))
	for _, issue := range response.Msg.GetIssues() {
		issues = append(issues, qualifyRelated(route, issue))
	}
	return connect.NewResponse(&v1.ListBoardPinsResponse{Issues: issues}), nil
}

// ListBoardPins returns one board's source-qualified pins without reordering them.
func (s *issueService) ListBoardPins(
	ctx context.Context,
	request *connect.Request[v1.ListBoardPinsRequest],
) (*connect.Response[v1.ListBoardPinsResponse], error) {
	return s.server.listBoardPins(ctx, request)
}
