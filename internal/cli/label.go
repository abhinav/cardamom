package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/Rovak/agents-clu/internal/store"
	"github.com/uptrace/bun"
)

type LabelCmd struct {
	Add       LabelAddCmd       `cmd:"" help:"Add labels to an issue."`
	Rm        LabelRmCmd        `cmd:"" aliases:"remove" help:"Remove labels from an issue."`
	Ls        LabelLsCmd        `cmd:"" aliases:"list" help:"List labels on an issue."`
	Propagate LabelPropagateCmd `cmd:"" help:"Push labels from a parent down to its children. Skips labels that are already present. Use --deep for transitive descendants."`
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

type LabelPropagateCmd struct {
	Parent string   `arg:"" name:"parent" help:"Parent issue ID."`
	Labels []string `arg:"" required:"" name:"label" help:"Label(s) to propagate."`
	Deep   bool     `name:"deep" aliases:"recursive" help:"Propagate to every transitive descendant, not just direct children."`
}

// labelPropagateResult is the JSON shape per child issue.
type labelPropagateResult struct {
	ID      string   `json:"id"`
	Added   []string `json:"added"`
	Skipped []string `json:"skipped"` // labels the child already had
}

type labelPropagateOut struct {
	Parent   string                 `json:"parent"`
	Deep     bool                   `json:"deep"`
	Labels   []string               `json:"labels"`
	Children []string               `json:"children"`
	Results  []labelPropagateResult `json:"results"`
}

func (c *LabelPropagateCmd) Run(r *runCtx) error {
	if len(c.Labels) == 0 {
		return fmt.Errorf("at least one label required")
	}
	for _, l := range c.Labels {
		if strings.TrimSpace(l) == "" {
			return fmt.Errorf("label cannot be empty")
		}
	}
	return withStore(r, func(s *store.Store) error {
		var children []string
		var err error
		if c.Deep {
			children, err = s.Descendants(r.ctx, c.Parent)
		} else {
			children, err = s.Children(r.ctx, c.Parent)
		}
		if err != nil {
			return err
		}

		// Pre-load existing labels so we can compute the precise
		// add/skip set per child without one query per child.
		existing, err := s.LoadLabels(r.ctx, children)
		if err != nil {
			return err
		}

		results := make([]labelPropagateResult, 0, len(children))
		// Per-label totals for the human summary.
		addedByLabel := make(map[string]int, len(c.Labels))
		skippedByLabel := make(map[string]int, len(c.Labels))
		for _, l := range c.Labels {
			addedByLabel[l] = 0
			skippedByLabel[l] = 0
		}

		// One transaction so a mid-propagation failure rolls everything
		// back — propagate is conceptually all-or-nothing.
		err = s.RunInTx(r.ctx, func(ctx context.Context, tx bun.Tx) error {
			for _, child := range children {
				have := map[string]bool{}
				for _, l := range existing[child] {
					have[l] = true
				}
				var toAdd, skipped []string
				for _, l := range c.Labels {
					if have[l] {
						skipped = append(skipped, l)
						skippedByLabel[l]++
					} else {
						toAdd = append(toAdd, l)
						addedByLabel[l]++
					}
				}
				if len(toAdd) > 0 {
					if _, err := store.AddLabelsTx(ctx, tx, child, toAdd); err != nil {
						return fmt.Errorf("%s: %w", child, err)
					}
				}
				results = append(results, labelPropagateResult{
					ID: child, Added: toAdd, Skipped: skipped,
				})
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Audit each child that actually gained labels. Done after the
		// tx commits (events are best-effort, like every other path).
		for _, res := range results {
			s.RecordLabeled(r.ctx, res.ID, res.Added)
		}

		if r.json {
			// Non-nil slices so `jq -r '.results[].skipped[]'` doesn't
			// trip on missing keys when a child had no overlap.
			for i := range results {
				if results[i].Added == nil {
					results[i].Added = []string{}
				}
				if results[i].Skipped == nil {
					results[i].Skipped = []string{}
				}
			}
			if children == nil {
				children = []string{}
			}
			return r.emitJSON(labelPropagateOut{
				Parent:   c.Parent,
				Deep:     c.Deep,
				Labels:   c.Labels,
				Children: children,
				Results:  results,
			})
		}

		scope := "direct child(ren)"
		if c.Deep {
			scope = "descendant(s)"
		}
		if len(children) == 0 {
			r.notice("no %s of %s\n", scope, c.Parent)
			return nil
		}
		r.notice("propagated to %d %s of %s:\n", len(children), scope, c.Parent)
		for _, l := range c.Labels {
			a, sk := addedByLabel[l], skippedByLabel[l]
			if sk > 0 {
				r.notice("  %s: %d added, %d already present\n", l, a, sk)
			} else {
				r.notice("  %s: %d added\n", l, a)
			}
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
		if r.json {
			if labels == nil {
				labels = []string{}
			}
			return r.emitJSON(labels)
		}
		if len(labels) == 0 {
			fmt.Fprintln(r.stdout, "(none)")
			return nil
		}
		fmt.Fprintln(r.stdout, strings.Join(labels, "\n"))
		return nil
	})
}
