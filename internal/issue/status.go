package issue

// StatusInput contains the execution values needed to present local status.
// Collection readers can supply this metadata without loading issue bodies.
type StatusInput struct {
	// Lifecycle is nil when the synchronized lifecycle is unresolved.
	Lifecycle *Lifecycle
	// Claimed reports active local custody.
	Claimed bool
	// Waiting reports a known waiting reason.
	Waiting bool
	// Conflicts identifies any disputed execution fields.
	Conflicts []ConflictReference
}

// DeriveStatus presents execution status without guessing disputed values.
// Dependency blockers are applied by relationship-aware readers afterward.
func DeriveStatus(input StatusInput) Status {
	if input.Lifecycle == nil || Unresolved(input.Conflicts, FieldWaiting) {
		return StatusUnresolved
	}
	if input.Claimed {
		return StatusInProgress
	}
	if input.Waiting {
		return StatusWaiting
	}
	switch *input.Lifecycle {
	case LifecycleOpen:
		if Unresolved(input.Conflicts, FieldKind) || Unresolved(input.Conflicts, FieldDependencies) || Unresolved(input.Conflicts, FieldCheckpoint) {
			return StatusUnresolved
		}
		return StatusReady
	case LifecycleClosed:
		return StatusClosed
	case LifecycleCancelled:
		return StatusCancelled
	default:
		return statusUnknown
	}
}
