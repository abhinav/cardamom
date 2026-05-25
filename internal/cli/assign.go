package cli

import "github.com/rovak/beadsv2/internal/store"

// AssignCmd is sugar for `bd update <id> --assignee <who>`.
// Pass an empty <who> to clear the assignee.
type AssignCmd struct {
	ID string `arg:"" help:"Issue ID."`
	To string `arg:"" optional:"" help:"Assignee name (empty clears)."`
}

func (c *AssignCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var f store.UpdateFields
		if c.To == "" {
			var none *string
			f.Assignee = &none
		} else {
			to := c.To
			ptr := &to
			f.Assignee = &ptr
		}
		i, err := s.Update(r.ctx, c.ID, f)
		if err != nil {
			return err
		}
		if c.To == "" {
			r.notice("unassigned %s\n", i.ID)
		} else {
			r.notice("assigned %s to %s\n", i.ID, c.To)
		}
		return nil
	})
}
