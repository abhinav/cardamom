package cli

import "github.com/rovak/beadsv2/internal/store"

type ListCmd struct {
	Status string `default:"open" enum:"open,in_progress,closed,all" help:"Filter by status."`
	Agent  string `short:"a" help:"Filter by agent lane."`
}

func (c *ListCmd) Run(r *runCtx) error {
	filter := c.Status
	if filter == "all" {
		filter = ""
	}
	return withStore(r, func(s *store.Store) error {
		issues, err := s.List(r.ctx, filter, agentPtr(c.Agent))
		if err != nil {
			return err
		}
		printIssues(r.stdout, issues)
		return nil
	})
}
