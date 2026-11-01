package configuration

import (
	"fmt"
	"slices"

	"go.abhg.dev/cardamom/internal/errkind"
)

// ConflictReference identifies the dispute that leaves a configuration field
// unresolved. A nil override with this reference does not mean inheritance.
type ConflictReference struct {
	// ID identifies the retained contender set.
	ID string `json:"id"` // required
	// Field identifies the unresolved configuration value.
	Field Field `json:"field"` // required
}

// RequireResolved rejects operations that consume disputed configuration.
// Operations that use only other fields can proceed with their known values.
func (c *Configuration) RequireResolved(fields ...Field) error {
	return requireResolved(c.Conflicts, fields)
}

// RequireResolved rejects a patch that would replace a disputed override.
// Such replacements must use conflict resolution to retain their provenance.
func (p *Patch) RequireResolved(conflicts []ConflictReference) error {
	return requireResolved(conflicts, p.Fields)
}

func requireResolved(conflicts []ConflictReference, fields []Field) error {
	for _, reference := range conflicts {
		if slices.Contains(fields, reference.Field) {
			return errkind.Errorf(errkind.Conflict, "configuration field %q is unresolved (conflict %s)", reference.Field.String(), reference.ID)
		}
	}
	return nil
}

// MarshalText encodes the stable field name for structured conflict references.
func (f Field) MarshalText() ([]byte, error) {
	name := f.String()
	if name == "" {
		return nil, fmt.Errorf("unknown conflict field %d", f)
	}
	return []byte(name), nil
}
