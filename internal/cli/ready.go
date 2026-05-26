package cli

import (
	"errors"
	"time"

	"github.com/rovak/clu/internal/store"
)

type ReadyCmd struct {
	N         int           `short:"n" default:"20" help:"Maximum number of issues."`
	Agent     string        `short:"a" help:"Lane to query (default: unassigned)."`
	Wait      bool          `help:"Block until at least one issue is ready."`
	Interval  time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
	Heartbeat bool          `name:"heartbeat" help:"While waiting, register as a live agent under -a's name."`
}

func (c *ReadyCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		caps := resolveAgent(r.dir, c.Agent)
		// Heartbeat is opt-in via --heartbeat. The `-a <name>` flag
		// doubles as the agent identity in this command.
		if c.Wait && c.Heartbeat {
			if c.Agent == "" {
				return errors.New("--heartbeat requires -a <name>")
			}
			cleanup, err := startHeartbeat(s, c.Agent, caps)
			if err != nil {
				return err
			}
			defer cleanup()
		}
		var (
			issues []store.Issue
			err    error
		)
		if c.Wait {
			issues, err = s.WaitReady(r.ctx, c.N, agentPtr(c.Agent), caps, c.Interval)
		} else {
			issues, err = s.Ready(r.ctx, c.N, agentPtr(c.Agent), caps)
		}
		if err != nil {
			return err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return err
		}
		// Ready by definition has no open blockers — pass nil so the
		// display short-circuits without an extra query.
		printIssues(r, issues, labels, nil)
		return nil
	})
}
