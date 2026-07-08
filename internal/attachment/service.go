package attachment

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

var (
	// ErrAttachmentTargetNotFound reports that the selected board or optional
	// origin issue does not exist.
	ErrAttachmentTargetNotFound = errors.New("attachment target not found")

	// ErrAttachmentNotFound reports that an attachment does not exist in the
	// selected board. Callers may match this outcome with errors.Is.
	ErrAttachmentNotFound = errors.New("attachment not found")

	// ErrAttachmentPageTokenInvalid reports that an attachment page token
	// cannot resume the selected listing.
	ErrAttachmentPageTokenInvalid = errors.New("invalid attachment page token")

	// ErrUploadNotFound reports that an upload session does not exist.
	ErrUploadNotFound = errors.New("attachment upload not found")

	// ErrUploadActorConflict reports that another actor owns the upload.
	ErrUploadActorConflict = errors.New("attachment upload belongs to another actor")

	// ErrUploadStateConflict reports that the upload's terminal state rejects
	// the requested operation.
	ErrUploadStateConflict = errors.New("attachment upload state conflict")

	// ErrUploadOffsetConflict reports a gap in sequential upload content.
	ErrUploadOffsetConflict = errors.New("attachment upload offset conflict")

	// ErrUploadChunkConflict reports that replayed content differs from staged
	// bytes.
	ErrUploadChunkConflict = errors.New("attachment upload chunk conflict")

	// ErrUploadDescriptorMismatch reports that staged content does not match a
	// client-declared size or digest.
	ErrUploadDescriptorMismatch = errors.New("attachment upload descriptor mismatch")
)

// UploadRepository persists the finite upload lifecycle.
type UploadRepository interface {
	// BeginUpload establishes one durable upload session.
	BeginUpload(context.Context, BeginUploadAdmission) (Upload, error)

	// GetUpload returns current progress or a terminal receipt.
	GetUpload(context.Context, GetUploadRequest) (Upload, error)

	// WriteChunk appends or replays bounded sequential content.
	WriteChunk(context.Context, WriteChunkRequest) (Upload, error)

	// CommitUpload publishes one attachment or returns its existing receipt.
	CommitUpload(context.Context, CommitUploadRequest) (Attachment, error)

	// AbortUpload abandons an active upload or returns its existing receipt.
	AbortUpload(context.Context, AbortUploadRequest) (Upload, error)
}

// MetadataRepository reads board-scoped attachment metadata.
type MetadataRepository interface {
	// GetAttachment returns one attachment with replica-local availability.
	GetAttachment(context.Context, GetRequest) (Attachment, error)

	// ListAttachments returns one stable page with replica-local availability.
	ListAttachments(context.Context, ListRequest) (Page, error)

	// ResolveAttachments returns input-aligned board-scoped reference results.
	ResolveAttachments(context.Context, ResolveRequest) ([]Resolution, error)
}

// ContentRepository opens fully verified immutable attachment content.
type ContentRepository interface {
	// OpenAttachmentContent returns active metadata and a caller-owned handle.
	OpenAttachmentContent(context.Context, OpenContentRequest) (OpenedContent, error)
}

// MaintenanceRepository owns attachment lifecycle and local byte maintenance.
type MaintenanceRepository interface {
	// RemoveAttachment creates or returns a permanent tombstone.
	RemoveAttachment(context.Context, RemoveRequest) (Attachment, error)

	// VerifyAttachment records one complete local integrity observation.
	VerifyAttachment(context.Context, VerifyRequest) (Verification, error)

	// CollectAttachments performs conservative local byte collection.
	CollectAttachments(context.Context, CollectRequest) (CollectionResult, error)
}

// Repository provides every finite persistence operation consumed by Service.
type Repository interface {
	UploadRepository
	MetadataRepository
	ContentRepository
	MaintenanceRepository
}

// Service owns caller-facing finite attachment operations.
type Service struct {
	repository    Repository            // required
	configuration ConfigurationResolver // required
}

// ConfigurationResolver resolves current attachment policy by board.
type ConfigurationResolver interface {
	// ResolveConfiguration returns one fully resolved per-operation snapshot.
	ResolveConfiguration(context.Context, board.ID) (configuration.Configuration, error)
}

// ServiceConfig supplies attachment service collaborators.
type ServiceConfig struct {
	// Repository owns durable attachment operations.
	Repository Repository // required

	// Configuration resolves live board policy. Nil uses built-in defaults.
	Configuration ConfigurationResolver
}

// NewService constructs finite attachment operations.
func NewService(config ServiceConfig) *Service {
	must.NotBeNilf(config.Repository, "attachment Repository is required")
	resolver := config.Configuration
	if resolver == nil {
		resolver = defaultConfiguration{}
	}
	return &Service{repository: config.Repository, configuration: resolver}
}

// BeginUpload validates and establishes one durable upload session.
func (s *Service) BeginUpload(
	ctx context.Context,
	request BeginUploadRequest,
) (Upload, error) {
	if err := request.Validate(); err != nil {
		return Upload{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	resolved, err := s.configuration.ResolveConfiguration(
		ctx,
		request.Association.BoardID(),
	)
	if err != nil {
		return Upload{}, err
	}
	maximum := resolved.Attachment.MaxBytes
	if request.ExpectedSizeBytes != nil && *request.ExpectedSizeBytes > maximum.Uint64() {
		return Upload{}, errkind.Errorf(
			errkind.InvalidInput,
			"attachment expected size %d exceeds %d bytes",
			*request.ExpectedSizeBytes,
			maximum.Uint64(),
		)
	}
	upload, err := s.repository.BeginUpload(ctx, BeginUploadAdmission{
		Request: request, MaximumSizeBytes: maximum,
	})
	return upload, classifyUploadError(err)
}

type defaultConfiguration struct{}

func (defaultConfiguration) ResolveConfiguration(
	context.Context,
	board.ID,
) (configuration.Configuration, error) {
	return configuration.Defaults(), nil
}

// GetUpload validates and returns upload progress or a terminal receipt.
func (s *Service) GetUpload(
	ctx context.Context,
	request GetUploadRequest,
) (Upload, error) {
	if err := request.Validate(); err != nil {
		return Upload{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	upload, err := s.repository.GetUpload(ctx, request)
	return upload, classifyUploadError(err)
}

// WriteChunk validates and appends or replays bounded sequential content.
func (s *Service) WriteChunk(
	ctx context.Context,
	request WriteChunkRequest,
) (Upload, error) {
	if err := request.Validate(); err != nil {
		return Upload{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	upload, err := s.repository.WriteChunk(ctx, request)
	return upload, classifyUploadError(err)
}

// CommitUpload validates and publishes an upload exactly once.
func (s *Service) CommitUpload(
	ctx context.Context,
	request CommitUploadRequest,
) (Attachment, error) {
	if err := request.Validate(); err != nil {
		return Attachment{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	attachment, err := s.repository.CommitUpload(ctx, request)
	return attachment, classifyUploadError(err)
}

// AbortUpload validates and abandons an active upload idempotently.
func (s *Service) AbortUpload(
	ctx context.Context,
	request AbortUploadRequest,
) (Upload, error) {
	if err := request.Validate(); err != nil {
		return Upload{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	upload, err := s.repository.AbortUpload(ctx, request)
	return upload, classifyUploadError(err)
}

// GetAttachment validates and returns board-scoped attachment metadata.
func (s *Service) GetAttachment(
	ctx context.Context,
	request GetRequest,
) (Attachment, error) {
	if err := request.Validate(); err != nil {
		return Attachment{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	value, err := s.repository.GetAttachment(ctx, request)
	return value, classifyAttachmentError(err)
}

// ListAttachments validates and returns one stable attachment page.
func (s *Service) ListAttachments(
	ctx context.Context,
	request ListRequest,
) (Page, error) {
	if err := request.Validate(); err != nil {
		return Page{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	page, err := s.repository.ListAttachments(ctx, request)
	return page, classifyAttachmentError(err)
}

// ResolveAttachments validates and resolves one bounded board-scoped reference
// batch while preserving request order and duplicates.
func (s *Service) ResolveAttachments(
	ctx context.Context,
	request ResolveRequest,
) ([]Resolution, error) {
	if err := request.Validate(); err != nil {
		return nil, errkind.Wrap(errkind.InvalidInput, err)
	}
	resolutions, err := s.repository.ResolveAttachments(ctx, request)
	return resolutions, classifyAttachmentError(err)
}

// OpenAttachmentContent validates and opens one active attachment as fully
// verified immutable content. The caller owns closing OpenedContent.Handle.
func (s *Service) OpenAttachmentContent(
	ctx context.Context,
	request OpenContentRequest,
) (OpenedContent, error) {
	if err := request.Validate(); err != nil {
		return OpenedContent{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	opened, err := s.repository.OpenAttachmentContent(ctx, request)
	return opened, classifyAttachmentError(err)
}

// RemoveAttachment validates and creates or returns a permanent tombstone.
func (s *Service) RemoveAttachment(
	ctx context.Context,
	request RemoveRequest,
) (Attachment, error) {
	if err := request.Validate(); err != nil {
		return Attachment{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	value, err := s.repository.RemoveAttachment(ctx, request)
	return value, classifyAttachmentError(err)
}

// VerifyAttachment validates and records one complete integrity observation.
func (s *Service) VerifyAttachment(
	ctx context.Context,
	request VerifyRequest,
) (Verification, error) {
	if err := request.Validate(); err != nil {
		return Verification{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	verification, err := s.repository.VerifyAttachment(ctx, request)
	return verification, classifyAttachmentError(err)
}

// CollectAttachments performs conservative local byte collection.
func (s *Service) CollectAttachments(
	ctx context.Context,
	request CollectRequest,
) (CollectionResult, error) {
	return s.repository.CollectAttachments(ctx, request)
}

func classifyUploadError(err error) error {
	switch {
	case errors.Is(err, ErrAttachmentTargetNotFound),
		errors.Is(err, ErrUploadNotFound):
		return errkind.Wrap(errkind.NotFound, err)
	case errors.Is(err, ErrUploadActorConflict),
		errors.Is(err, ErrUploadStateConflict),
		errors.Is(err, ErrUploadOffsetConflict),
		errors.Is(err, ErrUploadChunkConflict),
		errors.Is(err, ErrUploadDescriptorMismatch):
		return errkind.Wrap(errkind.Conflict, err)
	default:
		return err
	}
}

func classifyAttachmentError(err error) error {
	switch {
	case errors.Is(err, ErrAttachmentNotFound),
		errors.Is(err, ErrAttachmentContentMissing):
		return errkind.Wrap(errkind.NotFound, err)
	case errors.Is(err, ErrAttachmentPageTokenInvalid):
		return errkind.Wrap(errkind.InvalidInput, err)
	case errors.Is(err, ErrAttachmentRemoved),
		errors.Is(err, ErrAttachmentContentSizeMismatch),
		errors.Is(err, ErrAttachmentContentDigestMismatch):
		return errkind.Wrap(errkind.Conflict, err)
	default:
		return err
	}
}
