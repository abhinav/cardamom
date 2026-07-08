package attachment

import (
	"errors"
	"fmt"
	"io"

	"go.abhg.dev/cardamom/internal/board"
)

const (
	// MaxResolveAttachmentIDs is the largest attachment reference batch accepted
	// by ResolveAttachments.
	MaxResolveAttachmentIDs = 1000
)

var (
	// ErrAttachmentRemoved reports that attachment metadata is a permanent
	// tombstone. Callers may match this outcome with errors.Is.
	ErrAttachmentRemoved = errors.New("attachment removed")

	// ErrAttachmentContentMissing reports that an active attachment's immutable
	// content is unavailable in this store. Callers may match this outcome with
	// errors.Is.
	ErrAttachmentContentMissing = errors.New("attachment content missing")

	// ErrAttachmentContentSizeMismatch reports that local content has a different
	// size from the attachment descriptor. Callers may match this outcome with
	// errors.Is.
	ErrAttachmentContentSizeMismatch = errors.New("attachment content size mismatch")

	// ErrAttachmentContentDigestMismatch reports that size-valid local content
	// does not match the attachment digest. Callers may match this outcome with
	// errors.Is.
	ErrAttachmentContentDigestMismatch = errors.New("attachment content digest mismatch")
)

// ResolveRequest identifies an ordered, bounded attachment reference batch in
// one board. Duplicate attachment IDs are preserved.
type ResolveRequest struct {
	// BoardID identifies the board in which every attachment ID is resolved.
	BoardID board.ID // required

	// AttachmentIDs contains at most MaxResolveAttachmentIDs references. An
	// empty slice requests an empty result.
	AttachmentIDs []ID
}

// Validate verifies the board-scoped attachment reference batch.
func (r *ResolveRequest) Validate() error {
	if _, err := board.NewID(r.BoardID.String()); err != nil {
		return errors.New("attachment board ID required")
	}
	if len(r.AttachmentIDs) > MaxResolveAttachmentIDs {
		return fmt.Errorf(
			"attachment resolution exceeds %d IDs",
			MaxResolveAttachmentIDs,
		)
	}
	for _, id := range r.AttachmentIDs {
		if _, err := NewID(id.String()); err != nil {
			return errors.New("valid attachment ID required")
		}
	}
	return nil
}

// ResolutionState describes how one requested attachment ID resolved within
// the selected board.
type ResolutionState uint8

const (
	_ ResolutionState = iota

	// ResolutionUnknown means the attachment ID is unknown in the selected
	// board. An attachment owned by another board has this state.
	ResolutionUnknown

	// ResolutionActive means the attachment exists and is active.
	ResolutionActive

	// ResolutionRemoved means the attachment exists as a permanent tombstone.
	ResolutionRemoved
)

// Resolution is one result positionally aligned with ResolveRequest.AttachmentIDs.
type Resolution struct {
	// AttachmentID is the requested board-scoped attachment ID.
	AttachmentID ID // required

	// State distinguishes unknown, active, and removed metadata.
	State ResolutionState // required

	// Attachment contains known active or removed metadata and replica-local
	// availability. It is nil when State is ResolutionUnknown.
	Attachment *Attachment
}

// OpenContentRequest identifies one attachment whose immutable content should
// be opened for reading.
type OpenContentRequest struct {
	// BoardID identifies the attachment's owning board.
	BoardID board.ID // required

	// AttachmentID identifies active attachment metadata.
	AttachmentID ID // required
}

// Validate verifies the board-scoped attachment identity.
func (r *OpenContentRequest) Validate() error {
	return validateSelection(r.BoardID, r.AttachmentID)
}

// ContentHandle provides read-only random access to fully verified immutable
// attachment content. The caller that receives a handle owns Close.
type ContentHandle interface {
	io.Reader
	io.Seeker
	io.Closer
}

// OpenedContent contains active attachment metadata and its verified immutable
// content handle.
type OpenedContent struct {
	// Attachment is the active attachment whose availability is
	// BlobAvailabilityVerified.
	Attachment Attachment // required

	// Handle reads and seeks the immutable content. The caller owns Close.
	Handle ContentHandle // required
}
