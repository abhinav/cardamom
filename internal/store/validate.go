package store

import (
	"fmt"
	"strings"
)

// ValidStatuses are the statuses an issue may take. Source of truth for
// CLI enum validation and `clu statuses`.
//
//	open         not yet started
//	in_progress  claimed; an agent is working on it
//	closed       done successfully — unblocks downstream
//	cancelled    abandoned — downstream stays blocked (or is also
//	             cancelled in cascade by `clu cancel`)
var ValidStatuses = []string{"open", "in_progress", "closed", "cancelled"}

// ValidTypes are the canonical issue types. Source of truth for
// `cli types`. The schema does not enforce these — any string is
// allowed at the DB level — but the CLI uses this list for help text
// and discoverability.
var ValidTypes = []string{"task", "bug", "feature", "epic", "chore", "decision", "checkpoint"}

// Valid priority range, inclusive. 0 = highest, 4 = lowest. Five
// buckets keeps the urgency hierarchy meaningful without explicit
// label proliferation.
const (
	MinPriority = 0
	MaxPriority = 4
)

// ValidateStatus returns nil if s is in ValidStatuses, else an error
// wrapping ErrInvalid (so HTTP can map it to 400).
func ValidateStatus(s string) error {
	for _, v := range ValidStatuses {
		if v == s {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid status %q (valid: %s)", ErrInvalid, s, strings.Join(ValidStatuses, ", "))
}

// ValidateType returns nil if t is in ValidTypes, else an ErrInvalid.
func ValidateType(t string) error {
	for _, v := range ValidTypes {
		if v == t {
			return nil
		}
	}
	return fmt.Errorf("%w: invalid type %q (valid: %s)", ErrInvalid, t, strings.Join(ValidTypes, ", "))
}

// ValidatePriority returns nil if p is within [MinPriority, MaxPriority].
func ValidatePriority(p int) error {
	if p < MinPriority || p > MaxPriority {
		return fmt.Errorf("%w: invalid priority %d (must be %d..%d)", ErrInvalid, p, MinPriority, MaxPriority)
	}
	return nil
}
