package cli

import (
	"fmt"

	"github.com/rovak/beadsv2/internal/store"
)

type UpdateCmd struct {
	ID       string  `arg:"" help:"Issue ID."`
	Priority *int    `short:"p" help:"New priority (0=highest)."`
	Status   *string `help:"New status."`
	Assignee *string `help:"Set assignee."`
	Unassign bool    `help:"Clear assignee."`
	Agent    *string `short:"a" help:"Set agent lane."`
	NoAgent  bool    `help:"Clear agent lane."`
	Title    *string `help:"New title."`
}

func (c *UpdateCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var f store.UpdateFields
		f.Priority = c.Priority
		f.Status = c.Status
		f.Title = c.Title
		switch {
		case c.Unassign:
			var none *string
			f.Assignee = &none
		case c.Assignee != nil:
			f.Assignee = &c.Assignee
		}
		switch {
		case c.NoAgent:
			var none *string
			f.Agent = &none
		case c.Agent != nil:
			f.Agent = &c.Agent
		}
		i, err := s.Update(r.ctx, c.ID, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "updated %s\n", i.ID)
		return nil
	})
}
