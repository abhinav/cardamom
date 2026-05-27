package cli

import (
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

// resolveCheckpoint is the CLI-side wrapper around
// store.Store.ResolveCheckpoint. The engine itself lives in
// internal/store so HTTP handlers can call it without import cycles;
// this function only adds the CLI-specific formatting + notice output.
func resolveCheckpoint(r *runCtx, id, as string, pass bool, reason string) error {
	if as == "" {
		as = currentUser()
	}
	return withStore(r, func(s *store.Store) error {
		res, err := s.ResolveCheckpoint(r.ctx, id, as, pass, reason)
		if err != nil {
			return err
		}
		if res.Pass {
			r.notice("passed %s\n", res.Closed.ID)
			if r.json {
				return r.emitJSON(issueOut{Issue: *res.Closed, Labels: res.Labels})
			}
			return nil
		}
		for _, i := range res.Cancelled {
			r.notice("failed %s — %s\n", i.ID, i.Title)
		}
		if r.json {
			return r.emitJSON(res.Cancelled)
		}
		return nil
	})
}
