// Package information reports Cardamom store identity and inventory.
package information

import (
	"context"

	"go.abhg.dev/cardamom/internal/issue"
)

// Store identifies one physical Cardamom persistence boundary.
type Store struct {
	// Directory is the selected Cardamom store directory.
	Directory string

	// DatabasePath is the SQLite database inside Directory.
	DatabasePath string
}

// Schema identifies the persisted and running schema versions.
type Schema struct {
	// DatabaseVersion is the latest migration recorded by the store.
	DatabaseVersion int64

	// CodeVersion is the latest migration understood by the running program.
	CodeVersion int64
}

// Revision identifies the latest committed logical store change.
type Revision struct {
	// Current is the canonical store revision.
	Current int64
}

// IssueStatusCount reports one derived issue-status population.
type IssueStatusCount struct {
	// Status is the derived issue status being counted.
	Status issue.Status

	// Count is the number of selected-board issues with Status.
	Count int
}

// IssueInventory reports the selected board's issue population.
type IssueInventory struct {
	// Total is the number of issues in the selected board.
	Total int

	// ByStatus reports counts in issue.ValidStatuses order.
	ByStatus []IssueStatusCount
}

// Snapshot contains store-owned and board-owned information read through one
// retained persistence snapshot.
type Snapshot struct {
	// Schema identifies persisted and running schema versions.
	Schema Schema

	// Revision identifies the canonical store revision.
	Revision Revision

	// Issues reports the selected board's issue population.
	Issues IssueInventory
}

// Reader returns one persistence snapshot without exposing repository types.
type Reader interface {
	// Read returns schema, revision, and issue inventory from one snapshot.
	Read(context.Context) (Snapshot, error)
}
