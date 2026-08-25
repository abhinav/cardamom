package aggregate

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

func (s *Server) listLogs(
	ctx context.Context,
	request *connect.Request[v1.ListLogEntriesRequest],
) (*connect.Response[v1.ListLogEntriesResponse], error) {
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
	response, err := route.source.records.ListLogEntries(ctx, connect.NewRequest(&v1.ListLogEntriesRequest{
		IssueId: request.Msg.GetIssueId(), Direction: request.Msg.GetDirection(),
		Presentation: aggregatePresentation(route.source),
	}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no logs"))
	}
	entries := make([]*v1.LogEntry, 0, len(response.Msg.GetLogEntries()))
	for _, entry := range response.Msg.GetLogEntries() {
		value := proto.Clone(entry).(*v1.LogEntry)
		value.Source = proto.Clone(route.ref).(*v1.SourceRef)
		entries = append(entries, value)
	}
	return connect.NewResponse(&v1.ListLogEntriesResponse{
		LogEntries:      entries,
		AggregateStatus: &v1.AggregateStatus{Complete: true},
	}), nil
}

func (s *Server) getState(
	ctx context.Context,
	request *connect.Request[v1.GetStateRequest],
) (*connect.Response[v1.GetStateResponse], error) {
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
	response, err := route.source.records.GetState(ctx, connect.NewRequest(&v1.GetStateRequest{
		IssueId: request.Msg.GetIssueId(),
	}))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no state"))
	}
	return connect.NewResponse(proto.Clone(response.Msg).(*v1.GetStateResponse)), nil
}

type recordService struct {
	privatev1connect.UnimplementedRecordServiceHandler
	server *Server
}

func (s *recordService) ListLogEntries(
	ctx context.Context,
	request *connect.Request[v1.ListLogEntriesRequest],
) (*connect.Response[v1.ListLogEntriesResponse], error) {
	return s.server.listLogs(ctx, request)
}

func (s *recordService) GetState(
	ctx context.Context,
	request *connect.Request[v1.GetStateRequest],
) (*connect.Response[v1.GetStateResponse], error) {
	return s.server.getState(ctx, request)
}
