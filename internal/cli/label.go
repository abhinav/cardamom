package cli

import (
	"fmt"
	"strings"

	"github.com/rovak/clu/internal/store"
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
		// Honest count: actual inserts vs the (added==len) lie. Tells
		// the user when a "duplicate add" was effectively a no-op.
		skipped := len(c.Labels) - added
		if skipped > 0 {
			r.notice("added %d label(s) to %s (%d already present)\n", added, c.ID, skipped)
		} else {
			r.notice("added %d label(s) to %s\n", added, c.ID)
		}
		return nil
	})
}

type LabelRmCmd struct {
	ID     string   `arg:"" help:"Issue ID."`
	Labels []string `arg:"" required:"" help:"Label(s) to remove."`
}

func (c *LabelRmCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		removed, err := s.RemoveLabels(r.ctx, c.ID, c.Labels)
		if err != nil {
			return err
		}
		// Honest count: report actual removals, surface absent ones
		// so a script can tell whether the call changed anything.
		absent := len(c.Labels) - removed
		if absent > 0 {
			r.notice("removed %d label(s) from %s (%d not present)\n", removed, c.ID, absent)
		} else {
			r.notice("removed %d label(s) from %s\n", removed, c.ID)
		}
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
