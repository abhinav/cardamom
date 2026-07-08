package project

import (
	"context"
	"encoding/json"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
)

// IDStrategy selects the project issue ID allocation policy.
type IDStrategy uint8

const (
	// IDStrategyRandom allocates adaptive random issue suffixes.
	IDStrategyRandom IDStrategy = iota

	// IDStrategySequential allocates increasing decimal issue suffixes.
	IDStrategySequential
)

// NewIDStrategy parses the persisted project issue ID allocation policy.
func NewIDStrategy(value string) (IDStrategy, error) {
	switch value {
	case "random":
		return IDStrategyRandom, nil
	case "sequential":
		return IDStrategySequential, nil
	default:
		return 0, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: invalid ID strategy %q", value)
	}
}

// String returns the persisted ID strategy.
func (s IDStrategy) String() string {
	switch s {
	case IDStrategyRandom:
		return "random"
	case IDStrategySequential:
		return "sequential"
	default:
		panic(fmt.Sprintf("project: invalid ID strategy %d", s))
	}
}

// MarshalJSON preserves the textual ID strategy in structured output.
func (s IDStrategy) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Settings contains the validated project configuration established during
// initialization.
type Settings struct {
	// IDPrefix is prepended to allocated issue IDs.
	IDPrefix string

	// IDStrategy selects random or sequential issue ID allocation.
	IDStrategy IDStrategy
}

// StoreInitializationRequest supplies the namespace values needed to create or
// migrate a project's physical store.
type StoreInitializationRequest struct {
	// Dir contains the physical store files.
	Dir string

	// ProjectName is the initial user-visible project name.
	ProjectName string

	// BoardName is the initial user-visible board name. Nil creates no board.
	BoardName *string

	// ProjectIDPrefix is the optional validated project-level issue prefix
	// persisted for the initialized or retained namespace.
	ProjectIDPrefix *string
}

// InitializedNamespace identifies the project and optional board established
// while publishing a fresh store.
type InitializedNamespace struct {
	// Project is the namespace established by this initialization.
	Project *State

	// Board is the first coordination board, or nil when none was requested.
	Board *board.State
}

// StoreInitialization reports the database state established by one
// invocation. Namespace is non-nil only when the invocation creates the store.
type StoreInitialization struct {
	// DatabaseWritten reports whether this invocation published a new database.
	DatabaseWritten bool

	// SchemaVersion is the on-disk schema reached by the invocation.
	SchemaVersion int

	// Namespace identifies the project and optional board created with a fresh
	// database.
	Namespace *InitializedNamespace
}

// StoreInitializer creates or migrates one physical store and completes its
// initial project namespace before publication.
type StoreInitializer interface {
	InitializeStore(context.Context, StoreInitializationRequest) (StoreInitialization, error)
}
