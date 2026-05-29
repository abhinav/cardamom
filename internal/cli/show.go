package cli

import "github.com/rovak/clu/internal/store"

type ShowCmd struct {
	ID      string `arg:"" help:"Issue ID."`
	Context bool   `name:"context" help:"Also print the upstream dependency chain (descriptions, notes, comments) leading up to this issue."`
	Depth   int    `name:"context-depth" help:"Cap how far up the dependency chain --context walks (0 = unlimited)."`
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
		blocked, err := s.IDsBlocked(r.ctx, []string{i.ID})
		if err != nil {
			return err
		}
		return emitIssueWithContext(r, s, i, parents, blocks, labels, comments, blocked[i.ID], c.Context, c.Depth)
	})
}
