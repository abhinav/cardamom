package execution

import "go.abhg.dev/cardamom/internal/issue"

// Eligibility is an issue's current relationship to the execution and
// checkpoint pools. It keeps lifecycle, custody, waiting, dependency, and
// issue-kind policy consistent across every pool projection.
type Eligibility struct {
	kind  issue.Kind
	state eligibilityState
}

type eligibilityState uint8

const (
	eligibilityExcluded eligibilityState = iota
	eligibilityBlocked
	eligibilityAvailable
)

// EvaluateEligibility classifies one issue summary for execution pools.
func EvaluateEligibility(summary issue.Summary) (Eligibility, error) {
	kind, err := issue.NewKind(summary.Issue.Type)
	if err != nil {
		return Eligibility{}, err
	}
	lifecycle, err := issue.NewLifecycle(summary.Issue.Lifecycle)
	if err != nil {
		return Eligibility{}, err
	}
	eligibility := Eligibility{kind: kind}
	if lifecycle != issue.LifecycleOpen || summary.Issue.ActiveClaim != nil ||
		summary.Issue.Waiting != nil || kind == issue.KindRoutine {
		return eligibility, nil
	}
	if summary.Blocked {
		eligibility.state = eligibilityBlocked
		return eligibility, nil
	}
	eligibility.state = eligibilityAvailable
	return eligibility, nil
}

// ReadyForClaim reports whether an issue belongs in the unclaimed work pool.
func (e Eligibility) ReadyForClaim() bool {
	return e.state == eligibilityAvailable && e.kind.Executable()
}

// Blocked reports whether unresolved prerequisites prevent an otherwise
// visible non-routine issue from proceeding.
func (e Eligibility) Blocked() bool { return e.state == eligibilityBlocked }

// ActionableCheckpoint reports whether a checkpoint is ready for a pass or
// fail decision.
func (e Eligibility) ActionableCheckpoint() bool {
	return e.state == eligibilityAvailable && e.kind == issue.KindCheckpoint
}
