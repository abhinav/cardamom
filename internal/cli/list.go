package cli

import (
	"fmt"
	"time"

	"github.com/rovak/beadsv2/internal/store"
)

type ListCmd struct {
	Status string `default:"open" enum:"open,in_progress,closed,all" help:"Filter by stored status."`
	Agent  string `short:"a" help:"Filter by agent lane."`
	Type   string `short:"t" help:"Filter by type."`

	Label      []string `short:"l" sep:"," help:"Filter by labels (AND: must have ALL). Comma-separated or repeatable."`
	LabelAny   []string `name:"label-any" sep:"," help:"Filter by labels (OR: must have AT LEAST ONE)."`
	NoLabels   bool     `name:"no-labels" help:"Only issues with no labels."`
	NoAssignee bool     `name:"no-assignee" help:"Only issues with no assignee."`

	TitleContains string `name:"title-contains" help:"Filter by title substring (case-insensitive)."`

	PriorityMin *int `name:"priority-min" help:"Filter by minimum priority (inclusive, 0=highest)."`
	PriorityMax *int `name:"priority-max" help:"Filter by maximum priority (inclusive)."`

	CreatedAfter  string `name:"created-after" help:"Issues created on or after this date (YYYY-MM-DD or RFC3339)."`
	CreatedBefore string `name:"created-before" help:"Issues created on or before this date."`
	UpdatedAfter  string `name:"updated-after" help:"Issues updated on or after this date."`
	UpdatedBefore string `name:"updated-before" help:"Issues updated on or before this date."`

	ID []string `name:"id" sep:"," help:"Filter by specific issue IDs (comma-separated, e.g., bd-1,bd-5)."`

	Limit int `short:"n" default:"50" help:"Limit results (0 = unlimited)."`
}

func (c *ListCmd) Run(r *runCtx) error {
	f := store.ListFilter{
		Status:        c.Status,
		Agent:         agentPtr(c.Agent),
		Type:          c.Type,
		Labels:        c.Label,
		LabelsAny:     c.LabelAny,
		NoLabels:      c.NoLabels,
		NoAssignee:    c.NoAssignee,
		TitleContains: c.TitleContains,
		PriorityMin:   c.PriorityMin,
		PriorityMax:   c.PriorityMax,
		IDs:           c.ID,
		Limit:         c.Limit,
	}
	if f.Status == "all" {
		f.Status = ""
	}

	var err error
	if f.CreatedAfter, err = parseDate(c.CreatedAfter); err != nil {
		return fmt.Errorf("--created-after: %w", err)
	}
	if f.CreatedBefore, err = parseDate(c.CreatedBefore); err != nil {
		return fmt.Errorf("--created-before: %w", err)
	}
	if f.UpdatedAfter, err = parseDate(c.UpdatedAfter); err != nil {
		return fmt.Errorf("--updated-after: %w", err)
	}
	if f.UpdatedBefore, err = parseDate(c.UpdatedBefore); err != nil {
		return fmt.Errorf("--updated-before: %w", err)
	}

	return withStore(r, func(s *store.Store) error {
		issues, err := s.List(r.ctx, f)
		if err != nil {
			return err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return err
		}
		printIssues(r.stdout, issues, labels)
		return nil
	})
}

// parseDate accepts "YYYY-MM-DD" or RFC3339 and returns a Unix-epoch pointer.
// An empty string returns nil, nil (no filter on that dimension).
func parseDate(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.Unix()
			return &u, nil
		}
	}
	return nil, fmt.Errorf("invalid date %q (use YYYY-MM-DD or RFC3339)", s)
}

