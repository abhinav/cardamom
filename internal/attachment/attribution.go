package attachment

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/board"
)

// Invocation carries normalized mutation attribution for one attachment
// operation.
type Invocation struct {
	actor string
}

// NewInvocation normalizes attachment mutation attribution.
func NewInvocation(actor string) Invocation {
	return Invocation{actor: strings.TrimSpace(actor)}
}

// Actor returns the normalized mutation actor.
func (i *Invocation) Actor() string { return i.actor }

func (i *Invocation) validate() error {
	if i.actor == "" {
		return errors.New("attachment mutation actor required")
	}
	return nil
}

// Attribution records the actor, time, and canonical board revision of one
// logical attachment mutation.
type Attribution struct {
	// Actor identifies the mutation author.
	Actor string // required

	// At is the committed mutation time.
	At time.Time // required

	// Revision is the canonical board revision created by the mutation.
	Revision board.Revision // required
}

// Validate verifies complete mutation attribution.
func (a *Attribution) Validate() error {
	if a.Actor == "" || a.Actor != strings.TrimSpace(a.Actor) {
		return errors.New("attachment attribution actor required")
	}
	if a.At.IsZero() {
		return errors.New("attachment attribution time required")
	}
	if err := a.Revision.Validate(); err != nil {
		return fmt.Errorf("attachment attribution revision: %w", err)
	}
	return nil
}
