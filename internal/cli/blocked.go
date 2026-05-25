package cli

import "github.com/rovak/beadsv2/internal/store"

type BlockedCmd struct {
	N     int    `short:"n" default:"20" help:"Maximum number of issues."`
	Agent string `short:"a" help:"Lane to query (default: unassigned)."`
}

func (c *BlockedCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		issues, err := s.Blocked(r.ctx, c.N, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return err
		}
		printIssues(r, issues, labels)
		return nil
	})
}
