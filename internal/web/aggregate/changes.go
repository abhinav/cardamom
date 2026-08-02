package aggregate

import (
	"context"
	"errors"
	"sync"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

type changeService struct {
	privatev1connect.UnimplementedChangeServiceHandler
	server *Server
}

// aggregateChange carries one source-qualified invalidation to the single
// browser stream.
type aggregateChange struct {
	value *v1.WatchChangesResponse
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	changes := make(chan aggregateChange)
	var wait sync.WaitGroup
	for _, target := range targets {
		wait.Add(1)
		go func(target readTarget) {
			defer wait.Done()
			s.server.watchSource(ctx, target, changes)
		}(target)
	}
	go func() {
		wait.Wait()
		close(changes)
	}()
	for change := range changes {
		if err := stream.Send(change.value); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) watchSource(
	ctx context.Context,
	target readTarget,
	changes chan<- aggregateChange,
) {
	stream, err := target.source.changes.WatchChanges(ctx, connect.NewRequest(&v1.WatchChangesRequest{
		Scope: targetScope(target),
	}))
	if err != nil {
		s.sendHealth(ctx, target, changes)
		return
	}
	for stream.Receive() {
		value := proto.Clone(stream.Msg()).(*v1.WatchChangesResponse)
		if target.boardID != "" && value.GetBoardId() != target.boardID {
			continue
		}
		value.Source = sourceRefFromEntry(target.source)
		select {
		case changes <- aggregateChange{value: value}:
		case <-ctx.Done():
			return
		}
	}
	if ctx.Err() == nil {
		s.sendHealth(ctx, target, changes)
	}
}

func (s *Server) sendHealth(
	ctx context.Context,
	target readTarget,
	changes chan<- aggregateChange,
) {
	value := &v1.WatchChangesResponse{
		Source: sourceRefFromEntry(target.source),
		Health: &v1.SourceHealthEvent{
			Source:     sourceRefFromEntry(target.source),
			Health:     v1.SourceHealth_SOURCE_HEALTH_DEGRADED,
			Diagnostic: "change stream unavailable",
		},
	}
	select {
	case changes <- aggregateChange{value: value}:
	case <-ctx.Done():
	}
}
