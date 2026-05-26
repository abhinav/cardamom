package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
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
	ErrCheckpointBlocked   = errors.New("checkpoint has unresolved prerequisites")
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

// PendingCheckpoint bundles a checkpoint issue with the parsed
// payload the approval UI needs to render an actionable card. The
// embedded Issue carries description/title/timestamps; Approvers /
// Kind come from the cp:<id> KV row.
type PendingCheckpoint struct {
	Issue
	Kind      string   `json:"kind"`                // "approval" | "manual"
	Approvers []string `json:"approvers,omitempty"` // approval kind only
}

// PendingCheckpoints returns every open checkpoint issue with its
// parsed cp:<id> payload — the data feeding the /approvals page.
//
// Open = status in (open, in_progress) AND label "checkpoint:pending"
// is present. Checkpoints that have already been passed / failed lose
// the pending label, so this filter naturally excludes them.
//
// Issues that have no cp:<id> KV row (shouldn't happen for templated
// runs, but a hand-created checkpoint might) get an empty Approvers
// + Kind = "manual" so the UI can still render them.
//
// Ordered: oldest first (FIFO). Approvals are work-to-do; the oldest
// pending one is the most likely to be blocking something.
func (s *Store) PendingCheckpoints(ctx context.Context) ([]PendingCheckpoint, error) {
	var issues []Issue
	err := s.db.NewSelect().
		Model(&issues).
		Where("i.type = ?", "checkpoint").
		Where("i.status IN (?)", bun.In([]string{"open", "in_progress"})).
		Where("EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = i.id AND label = 'checkpoint:pending')").
		OrderExpr("i.created ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingCheckpoint, len(issues))
	for i, is := range issues {
		out[i] = PendingCheckpoint{Issue: is, Kind: "manual"}
		p, err := s.GetCheckpointPayload(ctx, is.ID)
		if err != nil {
			if errors.Is(err, ErrCheckpointNoPayload) {
				continue // already defaulted to manual
			}
			return nil, err
		}
		out[i].Kind = p.Kind
		out[i].Approvers = p.Approvers
	}
	return out, nil
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
	// Refuse to pass while the gate itself is still blocked. Approving
	// a gate whose `needs:` aren't done means downstream becomes ready
	// without the gated work having happened. Only enforced for pass —
	// fail on a still-blocked gate is fine (cancel cascade).
	if pass {
		blockedMap, err := s.IDsBlocked(ctx, []string{id})
		if err != nil {
			return CheckpointResult{}, err
		}
		if blockedMap[id] {
			return CheckpointResult{}, fmt.Errorf("%w: %s has unresolved prerequisites — wait for them to close, or `clu checkpoint fail` to cancel the gated chain", ErrCheckpointBlocked, id)
		}
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
	if _, err := s.AddLabels(ctx, id, []string{newLabel}); err != nil {
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
