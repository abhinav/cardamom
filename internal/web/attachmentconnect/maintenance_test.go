package attachmentconnect

import (
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.uber.org/mock/gomock"
)

func TestServiceMapsVerificationAndCollectionSummaries(t *testing.T) {
	digest, err := attachment.NewDigest("sha256:0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, err)
	descriptor := attachment.BlobDescriptor{Digest: digest, SizeBytes: 12}
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	verification := attachment.Verification{
		AttachmentID: attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa"),
		Blob:         descriptor,
		Availability: attachment.BlobAvailabilityDigestMismatch,
		ObservedAt:   now,
	}
	collection := attachment.CollectionResult{
		DryRun:         true,
		ExpiredStaging: attachment.CollectionSummary{Count: 2, Bytes: 30},
		OrphanBlobs:    attachment.CollectionSummary{Count: 3, Bytes: 45},
		IntegrityProblems: []attachment.IntegrityProblem{{
			BoardID:      board.ID("board-one"),
			AttachmentID: attachment.ID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa"),
			Blob:         descriptor,
			Availability: attachment.BlobAvailabilityMissing,
		}},
	}
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().VerifyAttachment(gomock.Any(), attachment.VerifyRequest{
		BoardID: "board-one", AttachmentID: verification.AttachmentID,
	}).Return(verification, nil)
	repository.EXPECT().CollectAttachments(
		gomock.Any(), attachment.CollectRequest{DryRun: true},
	).Return(collection, nil)
	client := newDomainTestClient(t, repository)

	verified, err := client.VerifyAttachment(t.Context(), connect.NewRequest(
		&privatev1.VerifyAttachmentRequest{
			BoardId: "board-one", AttachmentId: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
	))
	require.NoError(t, err)
	collected, err := client.CollectBlobs(t.Context(), connect.NewRequest(
		&privatev1.CollectBlobsRequest{DryRun: true},
	))

	require.NoError(t, err)
	assert.Equal(t, privatev1.BlobAvailability_BLOB_AVAILABILITY_DIGEST_MISMATCH, verified.Msg.GetVerification().GetAvailability())
	assert.Equal(t, now, verified.Msg.GetVerification().GetObservedAt().AsTime())
	result := collected.Msg.GetResult()
	assert.True(t, result.GetDryRun())
	assert.Equal(t, uint64(2), result.GetExpiredStaging().GetCount())
	assert.Equal(t, uint64(30), result.GetExpiredStaging().GetBytes())
	assert.Equal(t, uint64(3), result.GetOrphanBlobs().GetCount())
	assert.Equal(t, uint64(45), result.GetOrphanBlobs().GetBytes())
	require.Len(t, result.GetIntegrityProblems(), 1)
	assert.Equal(t, "board-one", result.GetIntegrityProblems()[0].GetBoardId())
	assert.Equal(t, privatev1.BlobAvailability_BLOB_AVAILABILITY_MISSING, result.GetIntegrityProblems()[0].GetAvailability())
}
