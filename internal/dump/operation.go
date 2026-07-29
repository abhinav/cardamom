package dump

import "fmt"

// ForceAuthorization controls whether publication may replace a recognized
// generated file whose body has changed since generation.
type ForceAuthorization int

const (
	// PreserveGenerated rejects modified generated files.
	PreserveGenerated ForceAuthorization = iota
	// ForceGenerated authorizes replacement of modified generated files.
	ForceGenerated
)

// Request describes one complete dump operation.
type Request struct {
	// Destination is the required output directory selected by the caller.
	Destination string

	// Selection identifies the issues represented by the dump.
	Selection Selection

	// Force authorizes replacement of modified recognized generated files.
	Force ForceAuthorization
}

// ExecutionResult describes the published dump and its generated file changes.
type ExecutionResult struct {
	// Destination is the output directory supplied in Request.
	Destination string

	// BoardID identifies the source board.
	BoardID string

	// Revision is the canonical source revision represented by the dump.
	Revision int64

	// Selection is the normalized selection represented by the dump.
	Selection Selection

	// Issues is the number of issue pages in the dump.
	Issues int

	// Written is the number of new or replaced generated files.
	Written int

	// Unchanged is the number of byte-identical generated files preserved.
	Unchanged int

	// Removed is the number of obsolete canonical issue paths removed.
	Removed int
}

// Publication is the selected rendered state supplied to a Publisher.
type Publication struct {
	// Destination is the collection directory to create or update.
	Destination string

	// Rendered contains only the files selected for this publication.
	Rendered RenderedDump

	// Force authorizes replacement of modified recognized generated files.
	Force ForceAuthorization
}

// PublicationResult counts generated file changes made by a Publisher.
type PublicationResult struct {
	// Written is the number of new or replaced generated files.
	Written int

	// Unchanged is the number of byte-identical generated files preserved.
	Unchanged int

	// Removed is the number of obsolete canonical issue paths removed.
	Removed int
}

// GeneratedFileKind identifies why a recognized generated file cannot be used.
type GeneratedFileKind int

const (
	_ GeneratedFileKind = iota

	// GeneratedFileModified identifies a generated file whose body digest changed.
	GeneratedFileModified
)

// GeneratedFileError reports damage to a recognized generated path.
type GeneratedFileError struct {
	// Path is the canonical collection-relative path.
	Path string

	// Kind classifies the ownership validation failure.
	Kind GeneratedFileKind
}

// Error describes the generated path and ownership failure.
func (e *GeneratedFileError) Error() string {
	switch e.Kind {
	case GeneratedFileModified:
		return fmt.Sprintf("generated file %q was modified after generation", e.Path)
	default:
		return fmt.Sprintf("generated file %q is invalid", e.Path)
	}
}

// PartialRecoveryError reports that publication and rollback both failed.
// RecoveryDirectory retains staged and backup files.
type PartialRecoveryError struct {
	// PublicationError is the operation that interrupted publication.
	PublicationError error

	// RollbackError describes state that rollback could not restore.
	RollbackError error

	// RecoveryDirectory retains staged next files and prior-file backups.
	RecoveryDirectory string
}

// Error describes the publication failure, rollback failure, and retained
// recovery state.
func (e *PartialRecoveryError) Error() string {
	return fmt.Sprintf(
		"publish generated files: %v; rollback: %v; destination may be partial; recovery state retained at %q",
		e.PublicationError, e.RollbackError, e.RecoveryDirectory,
	)
}

// Unwrap returns the publication error that initiated rollback.
func (e *PartialRecoveryError) Unwrap() error { return e.PublicationError }
