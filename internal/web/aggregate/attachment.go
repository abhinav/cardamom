package aggregate

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"google.golang.org/protobuf/proto"
)

func (s *Server) listAttachments(
	ctx context.Context,
	request *connect.Request[v1.ListAttachmentsRequest],
) (*connect.Response[v1.ListAttachmentsResponse], error) {
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
	response, err := route.source.attachments.ListAttachments(ctx, connect.NewRequest(
		&v1.ListAttachmentsRequest{
			BoardId:        route.boardID,
			IssueId:        request.Msg.IssueId,
			IncludeRemoved: request.Msg.GetIncludeRemoved(),
			PageSize:       request.Msg.GetPageSize(),
			PageToken:      request.Msg.PageToken,
		},
	))
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("source unavailable"))
	}
	if response == nil || response.Msg == nil {
		return nil, connect.NewError(connect.CodeInternal, errors.New("source returned no attachments"))
	}
	result := proto.Clone(response.Msg).(*v1.ListAttachmentsResponse)
	for _, attachment := range result.GetAttachments() {
		attachment.Source = sourceRefFromEntry(route.source)
	}
	return connect.NewResponse(result), nil
}

type attachmentService struct {
	privatev1connect.UnimplementedAttachmentServiceHandler
	server *Server
}

func (s *attachmentService) ListAttachments(
	ctx context.Context,
	request *connect.Request[v1.ListAttachmentsRequest],
) (*connect.Response[v1.ListAttachmentsResponse], error) {
	return s.server.listAttachments(ctx, request)
}
