package board

import (
	"strings"

	"go.abhg.dev/cardamom/internal/errkind"
)

// Selector identifies a board by stable identity or exact name.
type Selector struct {
	value string
}

// NewSelector parses a non-empty board selector.
func NewSelector(value string) (Selector, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Selector{}, errkind.Errorf(errkind.InvalidInput, "invalid project namespace: board selector required")
	}
	return Selector{value: value}, nil
}

// String returns the normalized external selector.
func (s Selector) String() string { return s.value }

// Matches reports whether candidate has the selected identity or exact name.
func (s Selector) Matches(candidate *State) bool {
	return candidate != nil && (s.value == candidate.ID().String() || s.value == candidate.Name())
}
