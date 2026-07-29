package issue

import (
	"strings"
)

// Invocation carries attribution resolved for one command or HTTP request.
// It is an immutable method argument rather than service or runtime state.
type Invocation struct{ actor string }

// NewInvocation normalizes command attribution supplied at a process boundary.
func NewInvocation(actor string) Invocation {
	return Invocation{actor: strings.TrimSpace(actor)}
}

// Actor returns the normalized invocation actor.
func (i Invocation) Actor() string { return i.actor }
