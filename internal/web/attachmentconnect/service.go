// Package attachmentconnect exposes attachment operations through Connect.
package attachmentconnect

import (
	"context"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/web"
)

// Operations supplies the finite attachment behavior exposed by
// AttachmentService.
type Operations interface {
	// BeginUpload establishes one durable upload session.
	BeginUpload(context.Context, attachment.BeginUploadRequest) (attachment.Upload, error)

	// WriteChunk appends or replays bounded sequential content.
	WriteChunk(context.Context, attachment.WriteChunkRequest) (attachment.Upload, error)

	// GetUpload returns current progress or a terminal receipt.
	GetUpload(context.Context, attachment.GetUploadRequest) (attachment.Upload, error)

	// CommitUpload publishes one attachment or returns its existing receipt.
	CommitUpload(context.Context, attachment.CommitUploadRequest) (attachment.Attachment, error)

	// AbortUpload abandons an active upload or returns its existing receipt.
	AbortUpload(context.Context, attachment.AbortUploadRequest) (attachment.Upload, error)

	// GetAttachment returns one attachment with replica-local availability.
	GetAttachment(context.Context, attachment.GetRequest) (attachment.Attachment, error)

	// ListAttachments returns one stable attachment page.
	ListAttachments(context.Context, attachment.ListRequest) (attachment.Page, error)

	// RemoveAttachment creates or returns a permanent tombstone.
	RemoveAttachment(context.Context, attachment.RemoveRequest) (attachment.Attachment, error)

	// VerifyAttachment records one complete local integrity observation.
	VerifyAttachment(context.Context, attachment.VerifyRequest) (attachment.Verification, error)

	// CollectAttachments performs conservative local byte collection.
	CollectAttachments(context.Context, attachment.CollectRequest) (attachment.CollectionResult, error)
}

//go:generate go tool mockgen -destination mock_repository_test.go -package attachmentconnect -typed -write_package_comment=false go.abhg.dev/cardamom/internal/attachment Repository

var _ Operations = (*attachment.Service)(nil)

// Service adapts attachment operations to generated AttachmentService RPCs.
type Service struct {
	privatev1connect.UnimplementedAttachmentServiceHandler
	operations Operations
}

var _ privatev1connect.AttachmentServiceHandler = (*Service)(nil)

// New constructs an AttachmentService handler from finite domain operations.
func New(operations Operations) *Service {
	must.NotBeNilf(operations, "attachmentconnect: attachment operations are required")
	return &Service{operations: operations}
}

func invalidInput(err error) error {
	return web.FromError(errkind.Wrap(errkind.InvalidInput, err))
}
