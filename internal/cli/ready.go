package cli

import (
	"time"

	"github.com/rovak/beadsv2/internal/store"
)

type ReadyCmd struct {
	N        int           `short:"n" default:"20" help:"Maximum number of issues."`
	Agent    string        `short:"a" help:"Lane to query (default: unassigned)."`
	Wait     bool          `help:"Block until at least one issue is ready."`
	Interval time.Duration `default:"250ms" help:"Poll interval when --wait is set."`
}

func (c *ReadyCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var (
			issues []store.Issue
			err    error
		)
		if c.Wait {
			issues, err = s.WaitReady(r.ctx, c.N, agentPtr(c.Agent), c.Interval)
		} else {
			issues, err = s.Ready(r.ctx, c.N, agentPtr(c.Agent))
		}
		if err != nil {
			return err
		}
		printIssues(r.stdout, issues)
		return nil
	})
}
