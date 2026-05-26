package cli

import (
	"errors"

	"github.com/rovak/clu/internal/store"
)

type UpdateCmd struct {
	ID            string  `arg:"" help:"Issue ID."`
	Priority      *int    `short:"p" help:"New priority (0=highest)."`
	Status        *string `help:"New status."`
	Assignee      *string `help:"Set assignee."`
	Unassign      bool    `help:"Clear assignee."`
	Agent         *string `short:"a" help:"Set agent lane."`
	NoAgent       bool    `help:"Clear agent lane."`
	Title         *string `help:"New title."`
	Description   *string `help:"Set description."`
	NoDescription bool    `name:"no-description" help:"Clear description."`
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
		switch {
		case c.NoDescription:
			var none *string
			f.Description = &none
		case c.Description != nil:
			f.Description = &c.Description
		}
		if !c.hasAnyField() {
			return errors.New("nothing to update — pass at least one of --title/--status/--priority/--assignee/--unassign/--agent/--no-agent/--description/--no-description")
		}
		i, err := s.Update(r.ctx, c.ID, f)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(issueOut{Issue: i})
		}
		r.notice("updated %s\n", i.ID)
		return nil
	})
}

// hasAnyField reports whether the caller actually passed something to
// update. Without this guard, `clu update <id>` lies — it reports
// success without changing anything.
func (c *UpdateCmd) hasAnyField() bool {
	return c.Priority != nil ||
		c.Status != nil ||
		c.Title != nil ||
		c.Assignee != nil || c.Unassign ||
		c.Agent != nil || c.NoAgent ||
		c.Description != nil || c.NoDescription
}
