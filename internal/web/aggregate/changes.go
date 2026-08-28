package aggregate

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

type changeService struct {
	privatev1connect.UnimplementedChangeServiceHandler
	server *Server
}

func (s *changeService) WatchChanges(
	ctx context.Context,
	request *connect.Request[v1.WatchChangesRequest],
	stream *connect.ServerStream[v1.WatchChangesResponse],
) error {
	if request == nil || request.Msg == nil {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("change request is required"))
	}
	targets, err := s.server.targets(request.Msg.GetScope())
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return connect.NewError(connect.CodeNotFound, errors.New("no boards match change scope"))
	}
	subscription := s.server.changes.subscribe(request.Msg.GetScope())
	defer subscription.close()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change, ok := <-subscription.changes:
			if !ok {
				return nil
			}
			if err := stream.Send(change); err != nil {
				return err
			}
		}
	}
}
