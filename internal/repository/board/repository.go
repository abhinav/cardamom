// Package board persists one board-scoped issue aggregate through finite
// domain operations and coherent read snapshots.
package board

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// Clock supplies the shared timestamp for one board operation.
type Clock interface {
	// Now returns the current wall-clock time.
	Now() time.Time
}

// Config selects one board and its issue identity policy.
type Config struct {
	// BoardID identifies the board whose aggregate Repository owns.
	BoardID domainboard.ID // required

	// IDPrefix is prepended to every newly allocated issue identity.
	IDPrefix string

	// IDStrategy is "random" or "sequential". Empty selects "random".
	IDStrategy string

	// Clock supplies operation timestamps. Nil uses the system clock.
	Clock Clock

	// Entropy supplies bytes for issue and log identities. Nil uses
	// crypto/rand.Reader.
	Entropy io.Reader
}

// Repository owns finite persistence operations for one selected board.
type Repository struct {
	store      *store.Store
	boardID    domainboard.ID
	idPrefix   string
	idStrategy string
	clock      Clock
	entropy    io.Reader
}

// New validates one selected-board repository configuration.
func New(persistence *store.Store, cfg Config) (*Repository, error) {
	if persistence == nil {
		return nil, errors.New("board store is required")
	}
	if _, err := domainboard.NewID(cfg.BoardID.String()); err != nil {
		return nil, fmt.Errorf("board ID: %w", err)
	}
	prefix := cfg.IDPrefix
	if prefix == "" {
		prefix = "an-"
	}
	parsedPrefix, err := configuration.NewPrefix(prefix)
	if err != nil {
		return nil, err
	}
	prefix = parsedPrefix.String()
	strategy := cfg.IDStrategy
	if strategy == "" {
		strategy = "random"
	}
	if strategy != "random" && strategy != "sequential" {
		return nil, fmt.Errorf("invalid issue ID strategy %q", strategy)
	}
	clock := cfg.Clock
	if clock == nil {
		clock = systemClock{}
	}
	entropy := cfg.Entropy
	if entropy == nil {
		entropy = rand.Reader
	}
	return &Repository{
		store: persistence, boardID: cfg.BoardID, idPrefix: prefix,
		idStrategy: strategy, clock: clock, entropy: entropy,
	}, nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// SQL operations stay private to this package while accepting both retained
// store snapshots and immediate writer transactions.
type queryScope interface {
	query.DBTX
}
