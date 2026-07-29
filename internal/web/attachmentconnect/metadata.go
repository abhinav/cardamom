package attachmentconnect

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetAttachment returns one board-scoped attachment.
func (s *Service) GetAttachment(
	ctx context.Context,
	request *connect.Request[privatev1.GetAttachmentRequest],
) (*connect.Response[privatev1.GetAttachmentResponse], error) {
	boardID, attachmentID, err := attachmentSelection(
		request.Msg.GetBoardId(),
		request.Msg.GetAttachmentId(),
	)
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.GetAttachment(ctx, attachment.GetRequest{
		BoardID: boardID, AttachmentID: attachmentID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	message, err := attachmentMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetAttachmentResponse{
		Attachment: message,
	}), nil
}

// ListAttachments returns one stable page of board-scoped attachments.
func (s *Service) ListAttachments(
	ctx context.Context,
	request *connect.Request[privatev1.ListAttachmentsRequest],
) (*connect.Response[privatev1.ListAttachmentsResponse], error) {
	boardID, err := board.NewID(request.Msg.GetBoardId())
	if err != nil {
		return nil, invalidInput(err)
	}
	var originIssueID *issue.ID
	if request.Msg.IssueId != nil {
		value, err := issue.NewID(request.Msg.GetIssueId())
		if err != nil {
			return nil, invalidInput(err)
		}
		originIssueID = &value
	}
	page, err := s.operations.ListAttachments(ctx, attachment.ListRequest{
		BoardID:        boardID,
		OriginIssueID:  originIssueID,
		IncludeRemoved: request.Msg.GetIncludeRemoved(),
		PageSize:       request.Msg.GetPageSize(),
		PageToken:      request.Msg.GetPageToken(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	response := &privatev1.ListAttachmentsResponse{
		Attachments: make([]*privatev1.Attachment, len(page.Attachments)),
	}
	for index, value := range page.Attachments {
		response.Attachments[index], err = attachmentMessage(value)
		if err != nil {
			return nil, web.FromError(err)
		}
	}
	if page.NextPageToken != "" {
		response.NextPageToken = &page.NextPageToken
	}
	return connect.NewResponse(response), nil
}

// RemoveAttachment creates or returns a permanent attachment tombstone.
func (s *Service) RemoveAttachment(
	ctx context.Context,
	request *connect.Request[privatev1.RemoveAttachmentRequest],
) (*connect.Response[privatev1.RemoveAttachmentResponse], error) {
	boardID, attachmentID, err := attachmentSelection(
		request.Msg.GetBoardId(),
		request.Msg.GetAttachmentId(),
	)
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.RemoveAttachment(ctx, attachment.RemoveRequest{
		Invocation:   attachment.NewInvocation(request.Msg.GetMutation().GetActor()),
		BoardID:      boardID,
		AttachmentID: attachmentID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	message, err := attachmentMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.RemoveAttachmentResponse{
		Attachment: message,
	}), nil
}

func attachmentSelection(
	boardValue string,
	attachmentValue string,
) (board.ID, attachment.ID, error) {
	boardID, err := board.NewID(boardValue)
	if err != nil {
		return "", "", err
	}
	attachmentID, err := attachment.NewID(attachmentValue)
	if err != nil {
		return "", "", err
	}
	return boardID, attachmentID, nil
}

func attachmentMessage(value attachment.Attachment) (*privatev1.Attachment, error) {
	lifecycle, err := attachmentLifecycle(value.Lifecycle)
	if err != nil {
		return nil, err
	}
	availability, err := blobAvailability(value.Availability)
	if err != nil {
		return nil, err
	}
	result := &privatev1.Attachment{
		Id: value.ID.String(), BoardId: value.Association.BoardID().String(),
		Blob:     blobDescriptorMessage(value.Blob),
		Filename: value.Filename.String(), MediaType: value.MediaType.String(),
		Lifecycle:    lifecycle,
		Availability: availability,
		Created:      attributionMessage(value.Created),
	}
	if issueID, ok := value.Association.OriginIssueID(); ok {
		text := issueID.String()
		result.IssueId = &text
	}
	if value.Removed != nil {
		result.Removed = attributionMessage(*value.Removed)
	}
	return result, nil
}

func blobDescriptorMessage(value attachment.BlobDescriptor) *privatev1.BlobDescriptor {
	return &privatev1.BlobDescriptor{
		Digest: value.Digest.String(), SizeBytes: value.SizeBytes,
	}
}

func attributionMessage(value attachment.Attribution) *privatev1.AttachmentAttribution {
	return &privatev1.AttachmentAttribution{
		Actor: value.Actor, At: timestamppb.New(value.At),
		Revision: uint64(value.Revision),
	}
}

func attachmentLifecycle(value attachment.Lifecycle) (privatev1.AttachmentLifecycle, error) {
	switch value {
	case attachment.LifecycleActive:
		return privatev1.AttachmentLifecycle_ATTACHMENT_LIFECYCLE_ACTIVE, nil
	case attachment.LifecycleRemoved:
		return privatev1.AttachmentLifecycle_ATTACHMENT_LIFECYCLE_REMOVED, nil
	default:
		return privatev1.AttachmentLifecycle_ATTACHMENT_LIFECYCLE_UNSPECIFIED,
			fmt.Errorf("unsupported attachment lifecycle %d", value)
	}
}

func blobAvailability(value attachment.BlobAvailability) (privatev1.BlobAvailability, error) {
	switch value {
	case attachment.BlobAvailabilityMissing:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_MISSING, nil
	case attachment.BlobAvailabilityPresentUnverified:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_PRESENT_UNVERIFIED, nil
	case attachment.BlobAvailabilitySizeMismatch:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_SIZE_MISMATCH, nil
	case attachment.BlobAvailabilityVerified:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_VERIFIED, nil
	case attachment.BlobAvailabilityDigestMismatch:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_DIGEST_MISMATCH, nil
	default:
		return privatev1.BlobAvailability_BLOB_AVAILABILITY_UNSPECIFIED,
			fmt.Errorf("unsupported attachment blob availability %d", value)
	}
}
