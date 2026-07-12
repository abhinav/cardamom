package cli

import "github.com/arjia-labs/clu/internal/store"

// DescribeCmd is sugar for `clu update <id> --description <text>`.
// Pass an empty <text> to clear the description.
type DescribeCmd struct {
	ID   string `arg:"" help:"Issue ID."`
	Text string `arg:"" optional:"" help:"Description text (empty clears)."`
}

func (c *DescribeCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var f store.UpdateFields
		if c.Text == "" {
			var none *string
			f.Description = &none
		} else {
			text := c.Text
			ptr := &text
			f.Description = &ptr
		}
		i, err := s.Update(r.ctx, c.ID, f)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		if c.Text == "" {
			r.notice("cleared description on %s\n", i.ID)
		} else {
			r.notice("described %s\n", i.ID)
		}
		return nil
	})
}
