package board

import (
	"errors"
	"fmt"
	"math"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

// PinLimit is a non-negative persisted board pin admission limit.
type PinLimit uint64

// NewPinLimit parses a pin limit representable by SQLite.
func NewPinLimit(value uint64) (PinLimit, error) {
	if value > math.MaxInt64 {
		return 0, fmt.Errorf("pin limit %d: must be at most %d", value, uint64(math.MaxInt64))
	}
	return PinLimit(value), nil
}

// Uint64 returns the configured count.
func (l PinLimit) Uint64() uint64 { return uint64(l) }

// ErrPinLimit reports that a board cannot admit another pinned issue under its
// effective configuration. Existing pins remain unchanged.
var ErrPinLimit = errkind.Wrap(errkind.Conflict, errors.New("board pin limit reached"))

// PinMutation reports one idempotent pin collection mutation.
type PinMutation struct {
	// Issue is the selected issue's current compact projection.
	Issue issue.Reference

	// Changed reports whether the ordered collection changed.
	Changed bool
}
