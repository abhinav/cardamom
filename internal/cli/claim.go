package cli

import (
	"errors"
	"time"

	"github.com/rovak/clu/internal/store"
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
			return reportClaimed(r, s, i)
		}
		agent := agentPtr(c.Agent)
		for {
			i, err := s.Claim(r.ctx, c.As, agent)
			if err == nil {
				return reportClaimed(r, s, i)
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

// reportClaimed prints the just-claimed issue in full (matches `cli show`).
// JSON mode emits the same structured payload as `show --json`. Human mode
// prefixes a "claimed …" notice for narrative.
func reportClaimed(r *runCtx, s *store.Store, i store.Issue) error {
	r.notice("claimed %s (%s)\n", i.ID, i.Title)
	parents, blocks, err := s.Deps(r.ctx, i.ID)
	if err != nil {
		return err
	}
	labels, err := s.LabelsForIssue(r.ctx, i.ID)
	if err != nil {
		return err
	}
	comments, err := s.Comments(r.ctx, i.ID)
	if err != nil {
		return err
	}
	blocked, err := s.IDsBlocked(r.ctx, []string{i.ID})
	if err != nil {
		return err
	}
	printIssue(r, i, parents, blocks, labels, comments, blocked[i.ID])
	return nil
}
