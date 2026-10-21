// Package boardclone defines local replication of a board between stores.
// A clone preserves shared identities and retained history, but starts a new
// writer and leaves replica-local custody behind. It does not update an
// existing replica or reconcile divergent state.
package boardclone

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/project"
)

// Repository publishes a complete board replica. Failure before commit leaves
// no destination board. A cleanup error after commit returns the populated
// result alongside the error. Existing boards and identity collisions conflict.
type Repository interface {
	Clone(context.Context, Request) (Result, error)
}

// Request identifies the board and its destination namespace.
type Request struct {
	// BoardID is preserved in the destination store.
	BoardID board.ID // required

	// ProjectID selects an existing destination project; projects are not cloned.
	ProjectID project.ID // required
}

// Result describes the committed destination replica.
type Result struct {
	// BoardID identifies the logical board in both stores.
	BoardID string `json:"board_id"`

	// Name is the preserved board name.
	Name string `json:"name"`

	// ProjectID identifies the destination project.
	ProjectID string `json:"project_id"`

	// Issues counts the current issue projection.
	Issues int `json:"issues"`

	// Mutations counts retained mutations from all source writers.
	Mutations int `json:"mutations"`

	// Blobs counts distinct current and historical attachment blobs.
	Blobs int `json:"blobs"`
}

// RequireRandomIDs rejects a configuration that cannot allocate independent
// public IDs on diverging replicas. Local store and project defaults remain
// local; callers must check the effective strategy on each side rather than
// materializing inherited configuration into board overrides.
func RequireRandomIDs(side string, strategy configuration.IDStrategy) error {
	if strategy != configuration.IDStrategyRandom {
		return errkind.Errorf(errkind.InvalidInput,
			"%s board uses sequential issue IDs; cloning requires adaptive-random IDs", side)
	}
	return nil
}
