package cli

import "github.com/rovak/beadsv2/internal/store"

type ShowCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *ShowCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		i, err := s.Get(r.ctx, c.ID)
		if err != nil {
			return err
		}
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
		printIssue(r, i, parents, blocks, labels, comments)
		return nil
	})
}
