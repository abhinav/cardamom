// Package project persists project and board namespaces.
package project

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.abhg.dev/cardamom/internal/repository/store"
)

var (
	_ board.Changes            = (*Repository)(nil)
	_ project.Projects         = (*Repository)(nil)
	_ projectcreation.Projects = (*Repository)(nil)
	_ board.Catalog            = (*Repository)(nil)
)

// Clock supplies durable namespace timestamps.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// IDSource supplies opaque store-unique namespace identities.
type IDSource interface {
	// NewID returns a new identity for kind or an entropy failure.
	NewID(kind string) (string, error)
}

// Config defines namespace identity and timestamp sources.
type Config struct {
	// Clock supplies durable namespace timestamps. Nil uses the system clock.
	Clock Clock

	// IDSource supplies store-unique identities. Nil uses crypto/rand.
	IDSource IDSource
}

// Repository owns finite project reads, board reads, and board mutations over
// one Store.
type Repository struct {
	// store owns transaction scopes for namespace operations.
	store *store.Store // required

	// clock supplies one timestamp source for namespace operations.
	clock func() time.Time // required

	// idSource supplies store-unique namespace identities.
	idSource IDSource // required
}

// New constructs a project namespace repository.
func New(persistence *store.Store, cfg Config) *Repository {
	must.NotBeNilf(persistence, "project Store is required")
	clock := time.Now
	if cfg.Clock != nil {
		clock = cfg.Clock.Now
	}
	idSource := cfg.IDSource
	if idSource == nil {
		idSource = randomIDs{}
	}
	return &Repository{store: persistence, clock: clock, idSource: idSource}
}

// randomIDs supplies namespace identities from cryptographic entropy.
type randomIDs struct{}

// NewID generates an opaque identity with a stable kind prefix.
func (randomIDs) NewID(kind string) (string, error) {
	value := make([]byte, 10)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return kind + "_" + hex.EncodeToString(value), nil
}
