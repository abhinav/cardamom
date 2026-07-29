package attachment

import (
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

// Lifecycle is the monotonic logical state of attachment metadata.
type Lifecycle uint8

const (
	_ Lifecycle = iota

	// LifecycleActive identifies attachment metadata available for same-board
	// references.
	LifecycleActive

	// LifecycleRemoved identifies a permanent attachment tombstone.
	LifecycleRemoved
)

var lifecycleNames = [...]string{"", "active", "removed"}

// NewLifecycle parses an attachment lifecycle.
func NewLifecycle(value string) (Lifecycle, error) {
	switch value {
	case "active":
		return LifecycleActive, nil
	case "removed":
		return LifecycleRemoved, nil
	default:
		return 0, fmt.Errorf("invalid attachment lifecycle %q", value)
	}
}

// String returns the stable attachment lifecycle name.
func (l Lifecycle) String() string {
	if int(l) >= len(lifecycleNames) {
		return ""
	}
	return lifecycleNames[l]
}

// Attachment is one immutable board-scoped file association and its current
// lifecycle and local availability observations.
type Attachment struct {
	// ID is the durable attachment handle.
	ID ID // required

	// Association identifies the owning board and optional originating issue.
	Association Association // required

	// Blob identifies the immutable content expected by this attachment.
	Blob BlobDescriptor // required

	// Filename is the portable presentation filename.
	Filename Filename // required

	// MediaType is the server-detected MIME media type.
	MediaType MediaType // required

	// Lifecycle is the monotonic logical attachment state.
	Lifecycle Lifecycle // required

	// Availability is the latest local content observation.
	Availability BlobAvailability // required

	// Created records attachment creation attribution.
	Created Attribution // required

	// Removed records tombstone attribution. It is present exactly when
	// Lifecycle is LifecycleRemoved.
	Removed *Attribution
}

// Validate verifies attachment metadata and lifecycle consistency.
func (a *Attachment) Validate() error {
	if id, err := NewID(a.ID.String()); err != nil || id != a.ID {
		return errors.New("attachment ID required")
	}
	if err := a.Association.Validate(); err != nil {
		return err
	}
	if err := a.Blob.Validate(); err != nil {
		return err
	}
	if filename, err := NewFilename(a.Filename.String()); err != nil || filename != a.Filename {
		return errors.New("valid attachment filename required")
	}
	if mediaType, err := NewMediaType(a.MediaType.String()); err != nil || mediaType != a.MediaType {
		return errors.New("valid attachment media type required")
	}
	if a.Lifecycle.String() == "" {
		return errors.New("attachment lifecycle required")
	}
	if a.Availability.String() == "" {
		return errors.New("attachment blob availability required")
	}
	if err := a.Created.Validate(); err != nil {
		return fmt.Errorf("attachment creation: %w", err)
	}

	switch a.Lifecycle {
	case LifecycleActive:
		if a.Removed != nil {
			return errors.New("active attachment cannot have removal attribution")
		}
	case LifecycleRemoved:
		if a.Removed == nil {
			return errors.New("removed attachment requires removal attribution")
		}
		if err := a.Removed.Validate(); err != nil {
			return fmt.Errorf("attachment removal: %w", err)
		}
		if a.Removed.Revision <= a.Created.Revision {
			return errors.New("attachment removal revision must follow creation")
		}
		if a.Removed.At.Before(a.Created.At) {
			return errors.New("attachment removal time must not precede creation")
		}
	}
	return nil
}

// GetRequest identifies one attachment within its owning board.
type GetRequest struct {
	// BoardID identifies the attachment's owning board.
	BoardID board.ID // required

	// AttachmentID identifies the attachment metadata.
	AttachmentID ID // required
}

// Validate verifies the board-scoped attachment identity.
func (r *GetRequest) Validate() error {
	return validateSelection(r.BoardID, r.AttachmentID)
}

// ListRequest selects one stable page of attachments from a board.
type ListRequest struct {
	// BoardID identifies the attachment's owning board.
	BoardID board.ID // required

	// OriginIssueID restricts results to attachments associated with one issue.
	// Nil includes attachments regardless of issue association.
	OriginIssueID *issue.ID

	// IncludeRemoved includes tombstoned attachment metadata.
	IncludeRemoved bool

	// PageSize is the requested maximum result count. Zero selects the server
	// default.
	PageSize uint32

	// PageToken resumes a prior stable page. Empty selects the first page.
	PageToken string
}

// Validate verifies the board and optional originating issue selector.
func (r *ListRequest) Validate() error {
	if _, err := board.NewID(r.BoardID.String()); err != nil {
		return errors.New("attachment board ID required")
	}
	if r.OriginIssueID != nil {
		if _, err := issue.NewID(r.OriginIssueID.String()); err != nil {
			return errors.New("attachment origin issue ID required")
		}
	}
	return nil
}

// Page contains one stable attachment page and an opaque continuation token.
type Page struct {
	// Attachments contains attachment metadata in stable repository order.
	Attachments []Attachment // required

	// NextPageToken resumes the next page. Empty means this is the last page.
	NextPageToken string
}

// RemoveRequest identifies one active attachment to tombstone. Replays return
// the existing tombstone without creating another revision.
type RemoveRequest struct {
	// Invocation identifies the removal actor.
	Invocation Invocation // required

	// BoardID identifies the attachment's owning board.
	BoardID board.ID // required

	// AttachmentID identifies the attachment metadata.
	AttachmentID ID // required
}

// Validate verifies removal attribution and board-scoped identity.
func (r *RemoveRequest) Validate() error {
	if err := r.Invocation.validate(); err != nil {
		return err
	}
	return validateSelection(r.BoardID, r.AttachmentID)
}

func validateSelection(boardID board.ID, attachmentID ID) error {
	if _, err := board.NewID(boardID.String()); err != nil {
		return errors.New("attachment board ID required")
	}
	if _, err := NewID(attachmentID.String()); err != nil {
		return errors.New("attachment ID required")
	}
	return nil
}
