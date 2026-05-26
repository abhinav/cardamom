package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// CheckpointPayload is the JSON shape stored in KV under "cp:<issue-id>"
// for checkpoint-type issues. Lifted out of the CLI so HTTP handlers can
// load and resolve checkpoints without depending on the CLI package.
type CheckpointPayload struct {
	Kind      string   `json:"kind"`                // "approval" | "manual"
	Approvers []string `json:"approvers,omitempty"` // approval kind only
}

// Sentinels used by ResolveCheckpoint so callers (CLI + HTTP) can map
// each failure mode to its own exit code / status code.
var (
	ErrNotCheckpoint       = errors.New("issue is not a checkpoint")
	ErrCheckpointClosed    = errors.New("checkpoint is already closed")
	ErrCheckpointNoPayload = errors.New("checkpoint has no wait payload")
	ErrNotApprover         = errors.New("user is not in approvers")
)

// GetCheckpointPayload loads the cp:<id> KV row for a checkpoint issue
// and unmarshals it. Returns ErrCheckpointNoPayload if the KV row is
// missing (the issue exists but was never wired up as a wait checkpoint).
func (s *Store) GetCheckpointPayload(ctx context.Context, id string) (CheckpointPayload, error) {
	raw, err := s.KVGet(ctx, "cp:"+id)
	if err != nil {
		if errors.Is(err, ErrKVNotFound) {
			return CheckpointPayload{}, ErrCheckpointNoPayload
		}
		return CheckpointPayload{}, err
	}
	var p CheckpointPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return CheckpointPayload{}, fmt.Errorf("checkpoint %s: invalid KV payload: %w", id, err)
	}
	return p, nil
}

// CheckpointResult bundles the outputs of ResolveCheckpoint so callers
// can format pass/fail uniformly without re-reading the issue.
type CheckpointResult struct {
	Pass      bool     // true if the checkpoint passed; false if failed
	Closed    Issue    // pass mode: the now-closed checkpoint issue
	Labels    []string // pass mode: labels on the closed issue (incl. checkpoint:passed)
	Cancelled []Issue  // fail mode: every issue cancelled (the checkpoint + downstream cascade)
}

// ResolveCheckpoint passes or fails a checkpoint issue:
//
//   - validates the issue exists, is type=checkpoint, and is not closed
//   - for pass+approval, checks `as` is in the approvers list
//   - swaps checkpoint:pending → checkpoint:passed | checkpoint:failed
//   - appends `reason` to notes (if non-empty)
//   - pass: MarkClosed the issue (unblocks downstream)
//     fail: Cancel cascade (terminal but not unblocking)
//
// Lifted from internal/cli/checkpoint.go so both the CLI and HTTP
// handlers go through the same engine.
func (s *Store) ResolveCheckpoint(ctx context.Context, id, as string, pass bool, reason string) (CheckpointResult, error) {
	issue, err := s.Get(ctx, id)
	if err != nil {
		return CheckpointResult{}, err
	}
	if issue.Type != "checkpoint" {
		return CheckpointResult{}, fmt.Errorf("%w: %s is a %s", ErrNotCheckpoint, id, issue.Type)
	}
	if issue.Status == "closed" {
		return CheckpointResult{}, fmt.Errorf("%w: %s", ErrCheckpointClosed, id)
	}
	payload, err := s.GetCheckpointPayload(ctx, id)
	if err != nil {
		return CheckpointResult{}, err
	}
	if pass && payload.Kind == "approval" && !containsString(payload.Approvers, as) {
		return CheckpointResult{}, fmt.Errorf("%w: %q not in %v", ErrNotApprover, as, payload.Approvers)
	}
	_ = s.RemoveLabels(ctx, id, []string{"checkpoint:pending"})
	newLabel := "checkpoint:passed"
	if !pass {
		newLabel = "checkpoint:failed"
	}
	if err := s.AddLabels(ctx, id, []string{newLabel}); err != nil {
		return CheckpointResult{}, err
	}
	if reason != "" {
		if _, err := s.AppendNote(ctx, id, reason); err != nil {
			return CheckpointResult{}, err
		}
	}
	if pass {
		closed, err := s.MarkClosed(ctx, id)
		if err != nil {
			return CheckpointResult{}, err
		}
		labels, _ := s.LabelsForIssue(ctx, id)
		return CheckpointResult{Pass: true, Closed: closed, Labels: labels}, nil
	}
	// Fail: cancel-cascade. Closing the gate would satisfy the `link`
	// edge from the downstream step and unblock it — opposite of what
	// "fail" should do. Cancelling is the terminal-but-not-unblocking
	// transition and cascades naturally.
	cancelled, err := s.Cancel(ctx, []string{id})
	if err != nil {
		return CheckpointResult{}, err
	}
	return CheckpointResult{Pass: false, Cancelled: cancelled}, nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
