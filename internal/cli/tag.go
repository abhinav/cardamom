package cli

import "github.com/rovak/beadsv2/internal/store"

// TagCmd is sugar for `clu label add <id> <labels...>`.
type TagCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Labels []string `arg:"" required:"" help:"Label(s) to add."`
}

func (c *TagCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.AddLabels(r.ctx, c.ID, c.Labels); err != nil {
			return err
		}
		r.notice("tagged %s with %d label(s)\n", c.ID, len(c.Labels))
		return nil
	})
}
