package attachment

import (
	"time"

	"go.abhg.dev/cardamom/internal/board"
)

// Verification is one complete local integrity observation for an attachment.
type Verification struct {
	// AttachmentID identifies the verified metadata.
	AttachmentID ID // required

	// Blob identifies the expected immutable content.
	Blob BlobDescriptor // required

	// Availability is the observed local content state after verification.
	Availability BlobAvailability // required

	// ObservedAt records when verification completed.
	ObservedAt time.Time // required
}

// VerifyRequest identifies one attachment for complete local integrity
// verification.
type VerifyRequest struct {
	// BoardID identifies the attachment's owning board.
	BoardID board.ID // required

	// AttachmentID identifies the attachment metadata.
	AttachmentID ID // required
}

// Validate verifies the board-scoped attachment identity.
func (r *VerifyRequest) Validate() error {
	return validateSelection(r.BoardID, r.AttachmentID)
}
