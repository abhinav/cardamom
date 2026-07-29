package attachment

import "go.abhg.dev/cardamom/internal/board"

// CollectionSummary reports a collected item count and total byte size.
type CollectionSummary struct {
	// Count is the number of collected items.
	Count uint64 // required

	// Bytes is the total size of collected content.
	Bytes uint64 // required
}

// IntegrityProblem reports expected retained content that collection found
// missing or invalid.
type IntegrityProblem struct {
	// BoardID identifies the board retaining the attachment metadata.
	BoardID board.ID // required

	// AttachmentID identifies the retained attachment.
	AttachmentID ID // required

	// Blob identifies the expected immutable content.
	Blob BlobDescriptor // required

	// Availability describes the missing or invalid local content.
	Availability BlobAvailability // required
}

// CollectionResult reports conservative local cleanup and retained-content
// observations. Collection never changes attachment metadata.
type CollectionResult struct {
	// DryRun reports whether collection planned changes without removing bytes.
	DryRun bool // required

	// ExpiredStaging reports inactive staged uploads selected for cleanup.
	ExpiredStaging CollectionSummary // required

	// OrphanBlobs reports content with no retained attachment metadata.
	OrphanBlobs CollectionSummary // required

	// IntegrityProblems reports retained metadata with missing or invalid bytes.
	IntegrityProblems []IntegrityProblem // required
}

// CollectRequest selects whether conservative local blob collection mutates
// storage or only reports its plan.
type CollectRequest struct {
	// DryRun reports collection results without removing bytes.
	DryRun bool
}
