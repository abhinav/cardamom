package issue

import (
	"time"

	"go.abhg.dev/cardamom/internal/errkind"
)

// CheckpointOutcome is the immutable result of a human checkpoint decision.
type CheckpointOutcome uint8

const (
	_ CheckpointOutcome = iota
	// CheckpointApproved completes a checkpoint successfully.
	CheckpointApproved
	// CheckpointDenied cancels a checkpoint and its dependent closure.
	CheckpointDenied
)

var checkpointOutcomeNames = [...]string{"", "approved", "denied"}

// NewCheckpointOutcome parses a persisted checkpoint outcome.
func NewCheckpointOutcome(value string) (CheckpointOutcome, error) {
	switch value {
	case "approved":
		return CheckpointApproved, nil
	case "denied":
		return CheckpointDenied, nil
	default:
		return 0, errkind.Errorf(
			errkind.InvalidInput,
			"invalid input: invalid checkpoint outcome %q",
			value,
		)
	}
}

// String returns the persisted checkpoint outcome.
func (o CheckpointOutcome) String() string {
	if int(o) >= len(checkpointOutcomeNames) {
		return ""
	}
	return checkpointOutcomeNames[o]
}

// CheckpointDecision is the persisted projection of one resolved checkpoint.
type CheckpointDecision struct {
	// Outcome records approval or denial.
	Outcome CheckpointOutcome `json:"outcome"`

	// Reason is the recorded Markdown rationale and may be empty.
	Reason string `json:"reason"`

	// DecidedAt is the decision time as Unix seconds.
	DecidedAt time.Time

	// Revision is the scalar revision that committed the decision.
	Revision int64 `json:"revision"`
}

// CheckpointDecisionView is the caller-facing decision projection.
type CheckpointDecisionView struct {
	// Outcome records approval or denial.
	Outcome string `json:"outcome"`

	// Reason is the recorded Markdown rationale and may be empty.
	Reason string `json:"reason"`

	// DecidedAt is the decision time as Unix seconds.
	DecidedAt int64 `json:"decided_at"`

	// Revision is the scalar revision that committed the decision.
	Revision int64 `json:"revision"`
}
