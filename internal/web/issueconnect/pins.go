package issueconnect

import (
	"context"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/web"
)

// ListBoardPins returns current issues in one board's insertion order.
func (s *Service) ListBoardPins(
	ctx context.Context,
	request *connect.Request[privatev1.ListBoardPinsRequest],
) (*connect.Response[privatev1.ListBoardPinsResponse], error) {
	boardID, operations, err := s.pinsForBoard(ctx, request.Msg.GetBoardId())
	if err != nil {
		return nil, web.FromError(err)
	}
	values, err := operations.List(ctx)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.References(boardID, values)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.ListBoardPinsResponse{
		Issues: converted,
	}), nil
}

// PinBoardIssue adds one issue to its owning board's pin order.
func (s *Service) PinBoardIssue(
	ctx context.Context,
	request *connect.Request[privatev1.PinBoardIssueRequest],
) (*connect.Response[privatev1.PinBoardIssueResponse], error) {
	boardID, operations, err := s.pinsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := operations.Pin(
		ctx,
		board.NewInvocation(request.Msg.GetContext().GetActor()),
		request.Msg.GetIssueId(),
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.References(boardID, []issue.Reference{result.Issue})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.PinBoardIssueResponse{
		Issue: converted[0], Changed: result.Changed,
	}), nil
}

// UnpinBoardIssue removes one issue from its owning board's pin order.
func (s *Service) UnpinBoardIssue(
	ctx context.Context,
	request *connect.Request[privatev1.UnpinBoardIssueRequest],
) (*connect.Response[privatev1.UnpinBoardIssueResponse], error) {
	boardID, operations, err := s.pinsForIssue(ctx, request.Msg.GetIssueId())
	if err != nil {
		return nil, web.FromError(err)
	}
	result, err := operations.Unpin(
		ctx,
		board.NewInvocation(request.Msg.GetContext().GetActor()),
		request.Msg.GetIssueId(),
	)
	if err != nil {
		return nil, web.FromError(err)
	}
	converted, err := s.views.References(boardID, []issue.Reference{result.Issue})
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.UnpinBoardIssueResponse{
		Issue: converted[0], Changed: result.Changed,
	}), nil
}

func (s *Service) pinsForBoard(
	ctx context.Context,
	value string,
) (board.ID, BoardPins, error) {
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
	operations, err := s.pins.Pins(boards[0].ID())
	return boardID, operations, err
}

func (s *Service) pinsForIssue(
	ctx context.Context,
	issueID string,
) (board.ID, BoardPins, error) {
	state, err := s.scope.BoardForIssue(ctx, issueID)
	if err != nil {
		return "", nil, err
	}
	operations, err := s.pins.Pins(state.ID())
	return state.ID(), operations, err
}
