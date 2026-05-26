package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rovak/clu/internal/store"
)

// CheckpointCmd is the parent for `clu checkpoint pass|fail`.
type CheckpointCmd struct {
	Pass CheckpointPassCmd `cmd:"" help:"Mark a checkpoint as passed (clears the wait, closes the issue, unblocks downstream)."`
	Fail CheckpointFailCmd `cmd:"" help:"Mark a checkpoint as failed (closes the issue with checkpoint:failed; downstream stays blocked)."`
}

type CheckpointPassCmd struct {
	ID     string `arg:"" help:"Checkpoint issue ID."`
	As     string `short:"a" name:"agent" default:"${user}" help:"Approver identity (defaults to $USER). Must match an entry in the approvers list."`
	Reason string `name:"reason" help:"Optional note appended to the issue when passing."`
}

func (c *CheckpointPassCmd) Run(r *runCtx) error {
	return resolveCheckpoint(r, c.ID, c.As, true, c.Reason)
}

type CheckpointFailCmd struct {
	ID     string `arg:"" help:"Checkpoint issue ID."`
	As     string `short:"a" name:"agent" default:"${user}" help:"Caller identity (defaults to $USER)."`
	Reason string `name:"reason" help:"Optional note appended to the issue when failing."`
}

func (c *CheckpointFailCmd) Run(r *runCtx) error {
	return resolveCheckpoint(r, c.ID, c.As, false, c.Reason)
}

// resolveCheckpoint is the shared engine for pass and fail. It loads
// the issue, validates it is an open checkpoint, checks approver match
// (pass only), swaps the pending → passed/failed label, optionally
// records a note, and closes the issue. `as` is the caller identity
// used for the approver check (defaults to the current OS user via
// kong's ${user} default).
func resolveCheckpoint(r *runCtx, id, as string, pass bool, reason string) error {
	if as == "" {
		as = currentUser()
	}
	return withStore(r, func(s *store.Store) error {
		issue, err := s.Get(r.ctx, id)
		if err != nil {
			return err
		}
		if issue.Type != "checkpoint" {
			return fmt.Errorf("%s is a %s, not a checkpoint", id, issue.Type)
		}
		if issue.Status == "closed" {
			return fmt.Errorf("%s is already closed", id)
		}
		payload, err := loadCheckpointPayload(r, s, id)
		if err != nil {
			return err
		}
		if pass && payload.Kind == "approval" {
			if !containsString(payload.Approvers, as) {
				return fmt.Errorf("user %q is not in approvers (%v) — pass --agent <approver> to override", as, payload.Approvers)
			}
		}
		// Swap label
		_ = s.RemoveLabels(r.ctx, id, []string{"checkpoint:pending"})
		newLabel := "checkpoint:passed"
		if !pass {
			newLabel = "checkpoint:failed"
		}
		if err := s.AddLabels(r.ctx, id, []string{newLabel}); err != nil {
			return err
		}
		if reason != "" {
			if _, err := s.AppendNote(r.ctx, id, reason); err != nil {
				return err
			}
		}
		if pass {
			closed, err := s.MarkClosed(r.ctx, id)
			if err != nil {
				return err
			}
			r.notice("passed %s\n", id)
			if r.json {
				labels, _ := s.LabelsForIssue(r.ctx, id)
				return r.emitJSON(issueOut{Issue: closed, Labels: labels})
			}
			return nil
		}
		// Fail: cancel-cascade. Closing the gate would satisfy the
		// `link` edge from the downstream step and unblock it — which
		// is exactly the opposite of what "fail" should do. Cancelling
		// is the terminal-but-not-unblocking transition, and it
		// naturally cascades to descendants (so the whole stuck-tail
		// is marked clearly instead of left dangling).
		cancelled, err := s.Cancel(r.ctx, []string{id})
		if err != nil {
			return err
		}
		for _, i := range cancelled {
			r.notice("failed %s — %s\n", i.ID, i.Title)
		}
		if r.json {
			return r.emitJSON(cancelled)
		}
		return nil
	})
}

func loadCheckpointPayload(r *runCtx, s *store.Store, id string) (checkpointPayload, error) {
	raw, err := s.KVGet(r.ctx, "cp:"+id)
	if err != nil {
		if errors.Is(err, store.ErrKVNotFound) {
			return checkpointPayload{}, fmt.Errorf("checkpoint %s has no wait payload (cp:%s missing in KV)", id, id)
		}
		return checkpointPayload{}, err
	}
	var p checkpointPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return checkpointPayload{}, fmt.Errorf("checkpoint %s: invalid KV payload: %w", id, err)
	}
	return p, nil
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
