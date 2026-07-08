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

// BeginAttachmentUpload establishes one durable resumable upload session.
func (s *Service) BeginAttachmentUpload(
	ctx context.Context,
	request *connect.Request[privatev1.BeginAttachmentUploadRequest],
) (*connect.Response[privatev1.BeginAttachmentUploadResponse], error) {
	domainRequest, err := beginUploadRequest(request.Msg)
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.BeginUpload(ctx, domainRequest)
	if err != nil {
		return nil, web.FromError(err)
	}
	upload, err := uploadMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.BeginAttachmentUploadResponse{
		Upload:                 upload,
		MaxChunkSizeBytes:      uint64(attachment.MaxChunkSizeBytes),
		MaxAttachmentSizeBytes: result.MaximumSizeBytes.Uint64(),
	}), nil
}

func beginUploadRequest(
	message *privatev1.BeginAttachmentUploadRequest,
) (attachment.BeginUploadRequest, error) {
	association, err := uploadAssociation(message.GetBoardId(), message.IssueId)
	if err != nil {
		return attachment.BeginUploadRequest{}, err
	}
	filename, err := attachment.NewFilename(message.GetFilename())
	if err != nil {
		return attachment.BeginUploadRequest{}, err
	}
	var expectedDigest *attachment.Digest
	if message.ExpectedDigest != nil {
		digest, err := attachment.NewDigest(message.GetExpectedDigest())
		if err != nil {
			return attachment.BeginUploadRequest{}, err
		}
		expectedDigest = &digest
	}
	var expectedSize *uint64
	if message.ExpectedSizeBytes != nil {
		expectedSize = new(message.GetExpectedSizeBytes())
	}
	return attachment.BeginUploadRequest{
		Invocation:        attachment.NewInvocation(message.GetMutation().GetActor()),
		Association:       association,
		Filename:          filename,
		ExpectedSizeBytes: expectedSize,
		ExpectedDigest:    expectedDigest,
	}, nil
}

// WriteAttachmentChunk appends or replays one sequential upload chunk.
func (s *Service) WriteAttachmentChunk(
	ctx context.Context,
	request *connect.Request[privatev1.WriteAttachmentChunkRequest],
) (*connect.Response[privatev1.WriteAttachmentChunkResponse], error) {
	uploadID, err := attachment.NewUploadID(request.Msg.GetUploadId())
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.WriteChunk(ctx, attachment.WriteChunkRequest{
		Invocation:     attachment.NewInvocation(request.Msg.GetMutation().GetActor()),
		UploadID:       uploadID,
		ExpectedOffset: request.Msg.GetExpectedOffset(),
		Content:        request.Msg.GetContent(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	upload, err := uploadMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.WriteAttachmentChunkResponse{
		Upload: upload,
	}), nil
}

// GetAttachmentUpload returns upload progress or a terminal receipt.
func (s *Service) GetAttachmentUpload(
	ctx context.Context,
	request *connect.Request[privatev1.GetAttachmentUploadRequest],
) (*connect.Response[privatev1.GetAttachmentUploadResponse], error) {
	uploadID, err := attachment.NewUploadID(request.Msg.GetUploadId())
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.GetUpload(ctx, attachment.GetUploadRequest{
		UploadID: uploadID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	upload, err := uploadMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.GetAttachmentUploadResponse{
		Upload: upload,
	}), nil
}

// CommitAttachmentUpload publishes an upload or returns its existing receipt.
func (s *Service) CommitAttachmentUpload(
	ctx context.Context,
	request *connect.Request[privatev1.CommitAttachmentUploadRequest],
) (*connect.Response[privatev1.CommitAttachmentUploadResponse], error) {
	uploadID, err := attachment.NewUploadID(request.Msg.GetUploadId())
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.CommitUpload(ctx, attachment.CommitUploadRequest{
		Invocation: attachment.NewInvocation(request.Msg.GetMutation().GetActor()),
		UploadID:   uploadID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	message, err := attachmentMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.CommitAttachmentUploadResponse{
		Attachment: message,
	}), nil
}

// AbortAttachmentUpload abandons an upload or returns its existing receipt.
func (s *Service) AbortAttachmentUpload(
	ctx context.Context,
	request *connect.Request[privatev1.AbortAttachmentUploadRequest],
) (*connect.Response[privatev1.AbortAttachmentUploadResponse], error) {
	uploadID, err := attachment.NewUploadID(request.Msg.GetUploadId())
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.AbortUpload(ctx, attachment.AbortUploadRequest{
		Invocation: attachment.NewInvocation(request.Msg.GetMutation().GetActor()),
		UploadID:   uploadID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	upload, err := uploadMessage(result)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.AbortAttachmentUploadResponse{
		Upload: upload,
	}), nil
}

func uploadAssociation(
	boardValue string,
	issueValue *string,
) (attachment.Association, error) {
	boardID, err := board.NewID(boardValue)
	if err != nil {
		return attachment.Association{}, err
	}
	if issueValue == nil {
		return attachment.NewBoardAssociation(boardID)
	}
	issueID, err := issue.NewID(*issueValue)
	if err != nil {
		return attachment.Association{}, err
	}
	return attachment.NewIssueAssociation(boardID, issueID)
}

func uploadMessage(value attachment.Upload) (*privatev1.AttachmentUpload, error) {
	state, err := uploadState(value.State)
	if err != nil {
		return nil, err
	}
	result := &privatev1.AttachmentUpload{
		Id: value.ID.String(), BoardId: value.Association.BoardID().String(),
		Filename: value.Filename.String(), Actor: value.Actor,
		State: state, AcceptedOffset: value.AcceptedOffset,
		ExpiresAt: timestamppb.New(value.ExpiresAt),
	}
	if issueID, ok := value.Association.OriginIssueID(); ok {
		text := issueID.String()
		result.IssueId = &text
	}
	result.ExpectedSizeBytes = value.ExpectedSizeBytes
	if value.ExpectedDigest != nil {
		text := value.ExpectedDigest.String()
		result.ExpectedDigest = &text
	}
	if value.Attachment != nil {
		result.Attachment, err = attachmentMessage(*value.Attachment)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func uploadState(value attachment.UploadState) (privatev1.AttachmentUploadState, error) {
	switch value {
	case attachment.UploadStateActive:
		return privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ACTIVE, nil
	case attachment.UploadStateCommitted:
		return privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_COMMITTED, nil
	case attachment.UploadStateAborted:
		return privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_ABORTED, nil
	case attachment.UploadStateExpired:
		return privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_EXPIRED, nil
	default:
		return privatev1.AttachmentUploadState_ATTACHMENT_UPLOAD_STATE_UNSPECIFIED,
			fmt.Errorf("unsupported attachment upload state %d", value)
	}
}
