package cli

import "github.com/rovak/clu/internal/store"

type BlockedCmd struct {
	Limit int    `short:"n" name:"limit" default:"20" help:"Limit results (0 = unlimited)."`
	Agent string `short:"a" help:"Lane to query (default: unassigned)."`
}

func (c *BlockedCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		issues, err := s.Blocked(r.ctx, c.Limit, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return err
		}
		// Every row here is blocked by definition — synthesise the map
		// rather than re-query.
		blocked := make(map[string]bool, len(issues))
		for _, i := range issues {
			blocked[i.ID] = true
		}
		printIssues(r, issues, labels, blocked)
		return nil
	})
}
