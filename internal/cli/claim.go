package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/rovak/clu/internal/store"
)

// ClaimCmd: one identity flag does everything.
//
//	clu claim                       → assignee = $USER, claims from the unassigned lane
//	clu claim -a code-reviewer      → claims as code-reviewer; lane filter = code-reviewer's
//	                                  (+ cap-routed unassigned work if declared in config)
//	clu claim --wait --heartbeat    → register as live in 'agent ls' while waiting
type ClaimCmd struct {
	Agent     string        `short:"a" name:"agent" help:"Agent identity. Sets the assignee and picks the lane to claim from. Unset = unassigned lane, assignee = $USER."`
	Wait      bool          `help:"Block until something is claimable."`
	Interval  time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
	Heartbeat bool          `name:"heartbeat" help:"While waiting, register --agent (or $USER) as a live agent so 'clu agent ls' shows this session active."`
	ID        string        `arg:"" optional:"" help:"Specific issue to claim; omit for next ready."`
}

func (c *ClaimCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		// assignee: who shows up in issue.assignee. Defaults to $USER
		// when no agent identity is given, so a human typing `clu claim`
		// at the terminal works.
		assignee := c.Agent
		if assignee == "" {
			assignee = currentUser()
		}
		if c.ID != "" {
			i, err := s.ClaimByID(r.ctx, c.ID, assignee)
			if err != nil {
				return err
			}
			return reportClaimed(r, s, i)
		}
		// Lane filter and capability lookup follow the explicit agent name
		// only. The default ($USER) intentionally doesn't filter — bare
		// `clu claim` should pull from the unassigned lane.
		var laneAgent *string
		var caps []string
		if c.Agent != "" {
			laneAgent = &c.Agent
			caps = resolveAgent(r.dir, c.Agent)
		}
		// Heartbeat is opt-in. Keys on whatever identity ends up in
		// assignee (so `clu claim --heartbeat` while logged in as rovak
		// shows up as "rovak" in agent ls).
		hbName := ""
		if c.Heartbeat && c.Wait {
			hbName = assignee
			cleanup, err := startHeartbeat(s, hbName, caps)
			if err != nil {
				return err
			}
			defer cleanup()
		}
		for {
			i, err := s.Claim(r.ctx, assignee, laneAgent, caps)
			if err == nil {
				return reportClaimed(r, s, i)
			}
			if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if !c.Wait {
				return noReadyError(r, s, laneAgent)
			}
			if _, err := s.WaitReady(r.ctx, 1, laneAgent, caps, c.Interval); err != nil {
				return err
			}
			heartbeatTick(s, hbName, caps)
		}
	})
}

// reportClaimed prints the just-claimed issue in full (matches `clu show`).
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

// noReadyError returns the "no ready issues" error with a hint about
// other lanes when the caller scoped to a named lane and the default
// lane has work. Without this, `claim -a foo` against issues created
// without -a returns a terse "no ready issues" with no clue why.
func noReadyError(r *runCtx, s *store.Store, laneAgent *string) error {
	if laneAgent == nil {
		// No lane scoping; nothing else to suggest.
		return errors.New("no ready issues")
	}
	// Count work in the default lane to know if there's a hint to give.
	defaultReady, err := s.Ready(r.ctx, 1, nil, nil)
	if err != nil || len(defaultReady) == 0 {
		return fmt.Errorf("no ready issues in lane %q", *laneAgent)
	}
	return fmt.Errorf("no ready issues in lane %q (%d issue(s) waiting in the default lane — drop --agent / -a to claim from there, or re-create with -a %s)",
		*laneAgent, len(defaultReady), *laneAgent)
}
