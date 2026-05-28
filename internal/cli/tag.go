package cli

import "github.com/rovak/clu/internal/store"

// TagCmd is sugar for `clu label add <id> <labels...>`.
type TagCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Labels []string `arg:"" required:"" help:"Label(s) to add."`
}

func (c *TagCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		added, err := s.AddLabels(r.ctx, c.ID, c.Labels)
		if err != nil {
			return err
		}
		if r.json {
			i, err := s.Get(r.ctx, c.ID)
			if err != nil {
				return err
			}
			labels, err := s.LabelsForIssue(r.ctx, c.ID)
			if err != nil {
				return err
			}
			return r.emitJSON(issueOut{Issue: i, Labels: labels})
		}
		skipped := len(c.Labels) - added
		if skipped > 0 {
			r.notice("tagged %s with %d label(s) (%d already present)\n", c.ID, added, skipped)
		} else {
			r.notice("tagged %s with %d label(s)\n", c.ID, added)
		}
		return nil
	})
}
