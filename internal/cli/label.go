package cli

import (
	"fmt"
	"strings"

	"github.com/rovak/beadsv2/internal/store"
)

type LabelCmd struct {
	Add LabelAddCmd `cmd:"" help:"Add labels to an issue."`
	Rm  LabelRmCmd  `cmd:"" aliases:"remove" help:"Remove labels from an issue."`
	Ls  LabelLsCmd  `cmd:"" aliases:"list" help:"List labels on an issue."`
}

type LabelAddCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Labels []string `arg:"" required:"" help:"Label(s) to add."`
}

func (c *LabelAddCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.AddLabels(r.ctx, c.ID, c.Labels); err != nil {
			return err
		}
		r.notice("added %d label(s) to %s\n", len(c.Labels), c.ID)
		return nil
	})
}

type LabelRmCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Labels []string `arg:"" required:"" help:"Label(s) to remove."`
}

func (c *LabelRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.RemoveLabels(r.ctx, c.ID, c.Labels); err != nil {
			return err
		}
		r.notice("removed %d label(s) from %s\n", len(c.Labels), c.ID)
		return nil
	})
}

type LabelLsCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *LabelLsCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		labels, err := s.LabelsForIssue(r.ctx, c.ID)
		if err != nil {
			return err
		}
		if len(labels) == 0 {
			fmt.Fprintln(r.stdout, "(none)")
			return nil
		}
		fmt.Fprintln(r.stdout, strings.Join(labels, "\n"))
		return nil
	})
}
