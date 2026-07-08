package attachment

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/configuration"
)

const (
	// MaxChunkSizeBytes is the largest upload chunk accepted by v1.
	MaxChunkSizeBytes = 4 << 20

	// StagingExpiry is the inactive lifetime of a staged upload.
	StagingExpiry = 24 * time.Hour

	// TerminalReceiptRetention is the recovery lifetime of a committed or
	// aborted upload receipt.
	TerminalReceiptRetention = 24 * time.Hour
)

// UploadID is an opaque durable upload-session identity.
type UploadID string

// NewUploadID parses a non-empty upload-session identity without whitespace.
func NewUploadID(value string) (UploadID, error) {
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", errors.New("attachment upload ID required")
	}
	return UploadID(value), nil
}

// String returns the opaque upload-session identity.
func (id UploadID) String() string { return string(id) }

// UploadState is the durable lifecycle of one resumable upload session.
type UploadState uint8

const (
	_ UploadState = iota

	// UploadStateActive accepts sequential chunks and may be committed or
	// aborted.
	UploadStateActive

	// UploadStateCommitted is a terminal receipt for one created attachment.
	UploadStateCommitted

	// UploadStateAborted is a terminal receipt for an explicitly abandoned
	// session.
	UploadStateAborted

	// UploadStateExpired is a terminal receipt for an inactive session whose
	// staging lifetime elapsed.
	UploadStateExpired
)

var uploadStateNames = [...]string{"", "active", "committed", "aborted", "expired"}

// NewUploadState parses a durable upload-session state.
func NewUploadState(value string) (UploadState, error) {
	for state := UploadStateActive; state <= UploadStateExpired; state++ {
		if state.String() == value {
			return state, nil
		}
	}
	return 0, fmt.Errorf("invalid attachment upload state %q", value)
}

// String returns the stable upload-session state name.
func (s UploadState) String() string {
	if int(s) >= len(uploadStateNames) {
		return ""
	}
	return uploadStateNames[s]
}

// Terminal reports whether the upload no longer accepts chunks or lifecycle
// changes.
func (s UploadState) Terminal() bool {
	return s == UploadStateCommitted || s == UploadStateAborted || s == UploadStateExpired
}

// Upload is the durable recovery view of one resumable upload session.
type Upload struct {
	// ID is the opaque upload-session identity.
	ID UploadID // required

	// Association identifies the target board and optional originating issue.
	Association Association // required

	// Filename is the portable filename selected when the session began.
	Filename Filename // required

	// ExpectedSizeBytes is the optional client-declared complete size.
	ExpectedSizeBytes *uint64

	// ExpectedDigest is the optional client-computed SHA-256 identity.
	ExpectedDigest *Digest

	// Actor owns the upload session.
	Actor string // required

	// State is the durable upload lifecycle.
	State UploadState // required

	// AcceptedOffset is the next sequential byte offset.
	AcceptedOffset uint64 // required

	// MaximumSizeBytes is the configuration limit admitted when this upload
	// began. Later configuration changes do not alter the active session.
	MaximumSizeBytes configuration.ByteLimit // required

	// ExpiresAt is the staging expiry for an active session and the receipt
	// expiry for a terminal session.
	ExpiresAt time.Time // required

	// Attachment is the one terminal attachment created by a committed upload.
	Attachment *Attachment
}

// Validate verifies upload metadata, bounds, and terminal-state consistency.
func (u *Upload) Validate() error {
	if id, err := NewUploadID(u.ID.String()); err != nil || id != u.ID {
		return errors.New("attachment upload ID required")
	}
	if err := u.Association.Validate(); err != nil {
		return err
	}
	if filename, err := NewFilename(u.Filename.String()); err != nil || filename != u.Filename {
		return errors.New("valid attachment filename required")
	}
	if u.Actor == "" || u.Actor != strings.TrimSpace(u.Actor) {
		return errors.New("attachment upload actor required")
	}
	if u.State.String() == "" {
		return errors.New("attachment upload state required")
	}
	if u.ExpiresAt.IsZero() {
		return errors.New("attachment upload expiry required")
	}
	if _, err := configuration.NewByteLimit(u.MaximumSizeBytes.Uint64()); err != nil {
		return errors.New("attachment upload maximum size required")
	}
	if u.ExpectedSizeBytes != nil && *u.ExpectedSizeBytes > u.MaximumSizeBytes.Uint64() {
		return fmt.Errorf("attachment expected size exceeds %d bytes", u.MaximumSizeBytes.Uint64())
	}
	if u.ExpectedDigest != nil {
		digest, err := NewDigest(u.ExpectedDigest.String())
		if err != nil || digest != *u.ExpectedDigest {
			return errors.New("valid attachment expected digest required")
		}
	}
	if u.AcceptedOffset > u.MaximumSizeBytes.Uint64() {
		return fmt.Errorf("attachment accepted offset exceeds %d bytes", u.MaximumSizeBytes.Uint64())
	}
	if u.ExpectedSizeBytes != nil && u.AcceptedOffset > *u.ExpectedSizeBytes {
		return errors.New("attachment accepted offset exceeds expected size")
	}

	if u.State != UploadStateCommitted {
		if u.Attachment != nil {
			return errors.New("uncommitted upload cannot contain an attachment")
		}
		return nil
	}
	if u.Attachment == nil {
		return errors.New("committed upload requires an attachment")
	}
	if err := u.Attachment.Validate(); err != nil {
		return fmt.Errorf("committed upload attachment: %w", err)
	}
	if u.Attachment.Lifecycle != LifecycleActive {
		return errors.New("committed upload attachment must be active")
	}
	if u.Attachment.Association != u.Association || u.Attachment.Filename != u.Filename {
		return errors.New("committed upload attachment does not match session")
	}
	if u.Attachment.Created.Actor != u.Actor {
		return errors.New("committed upload attachment actor does not match session")
	}
	if u.AcceptedOffset != u.Attachment.Blob.SizeBytes {
		return errors.New("committed upload offset does not match attachment size")
	}
	if u.ExpectedSizeBytes != nil && *u.ExpectedSizeBytes != u.Attachment.Blob.SizeBytes {
		return errors.New("committed upload attachment does not match expected size")
	}
	if u.ExpectedDigest != nil && *u.ExpectedDigest != u.Attachment.Blob.Digest {
		return errors.New("committed upload attachment does not match expected digest")
	}
	return nil
}

// BeginUploadRequest establishes one durable resumable upload session.
type BeginUploadRequest struct {
	// Invocation identifies the actor that owns the upload.
	Invocation Invocation // required

	// Association identifies the target board and optional originating issue.
	Association Association // required

	// Filename is the portable presentation filename.
	Filename Filename // required

	// ExpectedSizeBytes is the optional client-declared complete size.
	ExpectedSizeBytes *uint64

	// ExpectedDigest is the optional client-computed SHA-256 identity.
	ExpectedDigest *Digest
}

// BeginUploadAdmission combines caller input with the configuration maximum
// selected by Service for the lifetime of one upload session.
type BeginUploadAdmission struct {
	// Request is the validated caller request.
	Request BeginUploadRequest

	// MaximumSizeBytes is persisted with the upload session.
	MaximumSizeBytes configuration.ByteLimit
}

// Validate verifies upload ownership, metadata, and declared bounds.
func (r *BeginUploadRequest) Validate() error {
	if err := r.Invocation.validate(); err != nil {
		return err
	}
	if err := r.Association.Validate(); err != nil {
		return err
	}
	if filename, err := NewFilename(r.Filename.String()); err != nil || filename != r.Filename {
		return errors.New("valid attachment filename required")
	}
	if r.ExpectedDigest != nil {
		digest, err := NewDigest(r.ExpectedDigest.String())
		if err != nil || digest != *r.ExpectedDigest {
			return errors.New("valid attachment expected digest required")
		}
	}
	return nil
}

// WriteChunkRequest appends or idempotently replays one sequential upload
// chunk at ExpectedOffset.
type WriteChunkRequest struct {
	// Invocation identifies the upload owner.
	Invocation Invocation // required

	// UploadID identifies the durable upload session.
	UploadID UploadID // required

	// ExpectedOffset is the byte offset at which Content must begin.
	ExpectedOffset uint64 // required

	// Content is one non-empty chunk no larger than MaxChunkSizeBytes.
	Content []byte // required
}

// Validate verifies upload ownership, identity, offset, and chunk bounds.
func (r *WriteChunkRequest) Validate() error {
	if err := r.Invocation.validate(); err != nil {
		return err
	}
	if _, err := NewUploadID(r.UploadID.String()); err != nil {
		return err
	}
	if len(r.Content) == 0 {
		return errors.New("attachment chunk content required")
	}
	if len(r.Content) > MaxChunkSizeBytes {
		return fmt.Errorf("attachment chunk exceeds %d bytes", MaxChunkSizeBytes)
	}
	return nil
}

// GetUploadRequest identifies one durable upload session for recovery.
type GetUploadRequest struct {
	// UploadID identifies the durable upload session.
	UploadID UploadID // required
}

// Validate verifies the upload identity.
func (r *GetUploadRequest) Validate() error {
	_, err := NewUploadID(r.UploadID.String())
	return err
}

// CommitUploadRequest identifies an actor-owned upload to publish exactly
// once. Replays return the same attachment.
type CommitUploadRequest struct {
	// Invocation identifies the upload owner.
	Invocation Invocation // required

	// UploadID identifies the durable upload session.
	UploadID UploadID // required
}

// Validate verifies commit ownership and upload identity.
func (r *CommitUploadRequest) Validate() error {
	return validateUploadMutation(&r.Invocation, r.UploadID)
}

// AbortUploadRequest identifies an actor-owned uncommitted upload to abandon.
// Replays of an aborted upload return the same terminal receipt.
type AbortUploadRequest struct {
	// Invocation identifies the upload owner.
	Invocation Invocation // required

	// UploadID identifies the durable upload session.
	UploadID UploadID // required
}

// Validate verifies abort ownership and upload identity.
func (r *AbortUploadRequest) Validate() error {
	return validateUploadMutation(&r.Invocation, r.UploadID)
}

func validateUploadMutation(invocation *Invocation, uploadID UploadID) error {
	if err := invocation.validate(); err != nil {
		return err
	}
	_, err := NewUploadID(uploadID.String())
	return err
}
