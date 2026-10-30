package execution

import (
	"slices"

	"go.abhg.dev/cardamom/internal/issue"
)

// Eligibility is an issue's current relationship to the execution and
// checkpoint pools. It keeps lifecycle, custody, waiting, dependency, and
// issue-kind policy consistent across every pool projection.
type Eligibility struct {
	kind  *issue.Kind
	state eligibilityState
}

type eligibilityState uint8

const (
	eligibilityExcluded eligibilityState = iota
	eligibilityBlocked
	eligibilityAvailable
	eligibilityWaiting
	eligibilityConflicted
)

// EvaluateEligibility classifies one issue summary for execution pools.
func EvaluateEligibility(summary issue.Summary) (Eligibility, error) {
	var kind *issue.Kind
	if summary.Issue.Type != nil {
		value, err := issue.NewKind(*summary.Issue.Type)
		if err != nil {
			return Eligibility{}, err
		}
		kind = &value
	}
	var lifecycle *issue.Lifecycle
	if summary.Issue.Lifecycle != nil {
		value, err := issue.NewLifecycle(*summary.Issue.Lifecycle)
		if err != nil {
			return Eligibility{}, err
		}
		lifecycle = &value
	}
	return ClassifyEligibility(EligibilityInput{
		Kind: kind, Lifecycle: lifecycle, Claimed: summary.Issue.ActiveClaim != nil,
		Waiting: summary.Issue.Waiting != nil, Blocked: summary.Blocked,
		Conflicted: len(summary.Issue.Conflicts) > 0 || len(summary.EligibilityConflicts) > 0,
	}), nil
}

// EligibilityInput contains execution metadata without record bodies. Waiting
// records the presence of a known waiting reason.
type EligibilityInput struct {
	// Kind is nil when the workflow role is disputed.
	Kind *issue.Kind
	// Lifecycle is nil when the current lifecycle is disputed.
	Lifecycle *issue.Lifecycle
	// Claimed reports active local custody.
	Claimed bool
	// Waiting reports the presence of a known waiting reason.
	Waiting bool
	// Blocked reports at least one known unfinished prerequisite.
	Blocked bool
	// Conflicted covers disputed issue fields or inherited working context.
	Conflicted bool
}

// ClassifyEligibility applies pool policy to compact execution metadata.
func ClassifyEligibility(input EligibilityInput) Eligibility {
	eligibility := Eligibility{kind: input.Kind}
	// Known exclusions determine whether a conflict actually withheld work.
	// An unknown kind or lifecycle must reach the conflict classification rather
	// than disappearing from the pool as though it were known to be ineligible.
	if input.Lifecycle != nil && *input.Lifecycle != issue.LifecycleOpen || input.Claimed ||
		input.Kind != nil && *input.Kind == issue.KindRoutine {
		return eligibility
	}
	if input.Waiting {
		eligibility.state = eligibilityWaiting
		return eligibility
	}
	if input.Blocked {
		eligibility.state = eligibilityBlocked
		return eligibility
	}
	if input.Kind == nil || input.Lifecycle == nil || input.Conflicted {
		eligibility.state = eligibilityConflicted
		return eligibility
	}
	eligibility.state = eligibilityAvailable
	return eligibility
}

// ReadyForClaim reports whether an issue belongs in the unclaimed work pool.
func (e *Eligibility) ReadyForClaim() bool {
	return e.state == eligibilityAvailable && e.kind != nil && e.kind.Executable()
}

// WaitingForContinuation reports whether an executable issue is waiting for
// its reason to be reconsidered before work continues.
func (e *Eligibility) WaitingForContinuation() bool {
	return e.state == eligibilityWaiting && e.kind != nil && e.kind.Executable()
}

// SkippedForConflict reports work that automatic claiming could select if its
// unresolved fields or inherited eligibility inputs were resolved. Known
// checkpoints are not executable work and do not contribute to this count.
func (e *Eligibility) SkippedForConflict() bool {
	return e.state == eligibilityConflicted && (e.kind == nil || e.kind.Executable())
}

// Blocked reports whether unresolved prerequisites prevent an otherwise
// visible non-routine issue from proceeding.
func (e *Eligibility) Blocked() bool { return e.state == eligibilityBlocked }

// ActionableCheckpoint reports whether a checkpoint is ready for a pass or
// fail decision.
func (e *Eligibility) ActionableCheckpoint() bool {
	return e.state == eligibilityAvailable && e.kind != nil && *e.kind == issue.KindCheckpoint
}

// MatchesKnownClaimLabels reports whether known memberships permit the label
// filter. Disputed requested memberships remain candidates for the conflict
// diagnostic rather than being treated as known absence.
func MatchesKnownClaimLabels(summary issue.Summary, command ClaimNext) bool {
	possible := func(label string) bool {
		if slices.Contains(summary.Labels, label) {
			return true
		}
		for _, conflict := range summary.Issue.Conflicts {
			if conflict.Field == issue.FieldLabels && (conflict.Key == "" || conflict.Key == label) {
				return true
			}
		}
		return false
	}
	for _, label := range command.LabelsAll {
		if !possible(label.String()) {
			return false
		}
	}
	if len(command.LabelsAny) > 0 {
		matched := false
		for _, label := range command.LabelsAny {
			matched = matched || possible(label.String())
		}
		if !matched {
			return false
		}
	}
	for _, label := range command.LabelsNone {
		if slices.Contains(summary.Labels, label.String()) {
			return false
		}
	}
	return true
}
