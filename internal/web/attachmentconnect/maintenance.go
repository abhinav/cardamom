package attachmentconnect

import (
	"context"

	"connectrpc.com/connect"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VerifyAttachment performs complete local attachment integrity verification.
func (s *Service) VerifyAttachment(
	ctx context.Context,
	request *connect.Request[privatev1.VerifyAttachmentRequest],
) (*connect.Response[privatev1.VerifyAttachmentResponse], error) {
	boardID, attachmentID, err := attachmentSelection(
		request.Msg.GetBoardId(),
		request.Msg.GetAttachmentId(),
	)
	if err != nil {
		return nil, invalidInput(err)
	}
	result, err := s.operations.VerifyAttachment(ctx, attachment.VerifyRequest{
		BoardID: boardID, AttachmentID: attachmentID,
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	availability, err := blobAvailability(result.Availability)
	if err != nil {
		return nil, web.FromError(err)
	}
	return connect.NewResponse(&privatev1.VerifyAttachmentResponse{
		Verification: &privatev1.AttachmentVerification{
			AttachmentId: result.AttachmentID.String(),
			Blob:         blobDescriptorMessage(result.Blob),
			Availability: availability,
			ObservedAt:   timestamppb.New(result.ObservedAt),
		},
	}), nil
}

// CollectBlobs collects expired staging and true orphan blob content.
func (s *Service) CollectBlobs(
	ctx context.Context,
	request *connect.Request[privatev1.CollectBlobsRequest],
) (*connect.Response[privatev1.CollectBlobsResponse], error) {
	result, err := s.operations.CollectAttachments(ctx, attachment.CollectRequest{
		DryRun: request.Msg.GetDryRun(),
	})
	if err != nil {
		return nil, web.FromError(err)
	}
	problems := make([]*privatev1.AttachmentIntegrityProblem, len(result.IntegrityProblems))
	for index, problem := range result.IntegrityProblems {
		availability, err := blobAvailability(problem.Availability)
		if err != nil {
			return nil, web.FromError(err)
		}
		problems[index] = &privatev1.AttachmentIntegrityProblem{
			BoardId:      problem.BoardID.String(),
			AttachmentId: problem.AttachmentID.String(),
			Blob:         blobDescriptorMessage(problem.Blob),
			Availability: availability,
		}
	}
	return connect.NewResponse(&privatev1.CollectBlobsResponse{
		Result: &privatev1.BlobCollectionResult{
			DryRun:            result.DryRun,
			ExpiredStaging:    collectionSummaryMessage(result.ExpiredStaging),
			OrphanBlobs:       collectionSummaryMessage(result.OrphanBlobs),
			IntegrityProblems: problems,
		},
	}), nil
}

func collectionSummaryMessage(value attachment.CollectionSummary) *privatev1.CollectionSummary {
	return &privatev1.CollectionSummary{Count: value.Count, Bytes: value.Bytes}
}
