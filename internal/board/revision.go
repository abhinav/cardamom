package board

import "go.abhg.dev/cardamom/internal/errkind"

// Revision identifies one canonical board state.
type Revision int64

// Validate verifies that revision is non-negative.
func (r Revision) Validate() error {
	if r < 0 {
		return errkind.Errorf(errkind.InvalidInput, "invalid board: revision must not be negative")
	}
	return nil
}
