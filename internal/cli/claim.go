package cli

import (
	"errors"
	"time"

	"github.com/rovak/beadsv2/internal/store"
)

type ClaimCmd struct {
	As       string        `default:"${user}" help:"Assignee name (defaults to current user)."`
	Agent    string        `short:"a" help:"Lane to claim from (default: unassigned)."`
	Wait     bool          `help:"Block until something is claimable in this lane."`
	Interval time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
	ID       string        `arg:"" optional:"" help:"Specific issue to claim; omit for next ready."`
}

func (c *ClaimCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if c.ID != "" {
			i, err := s.ClaimByID(r.ctx, c.ID, c.As)
			if err != nil {
				return err
			}
			r.notice("claimed %s (%s)\n", i.ID, i.Title)
			return nil
		}
		agent := agentPtr(c.Agent)
		for {
			i, err := s.Claim(r.ctx, c.As, agent)
			if err == nil {
				r.notice("claimed %s (%s)\n", i.ID, i.Title)
				return nil
			}
			if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if !c.Wait {
				return errors.New("no ready issues")
			}
			// Wait for something to become ready, then retry the claim.
			// Another agent may steal it between Wait and Claim — that's
			// fine, we'll just block again.
			if _, err := s.WaitReady(r.ctx, 1, agent, c.Interval); err != nil {
				return err
			}
		}
	})
}
