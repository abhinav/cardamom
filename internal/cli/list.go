package cli

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/arjia-labs/clu/internal/store"
)

// listFilterFlags is the kong-tagged set of flags that build a
// store.ListFilter. Embedded by ListCmd and CountCmd so both commands
// stay in lock-step. Limit is intentionally separate from the embed:
// list cares about it, count does not.
type listFilterFlags struct {
	Status []string `default:"open,in_progress" sep:"," enum:"open,in_progress,closed,cancelled,active,all" help:"Filter by status. Comma-separated; 'active' = open+in_progress; 'all' disables the filter."`
	Agent  string   `short:"a" help:"Filter by assignee."`
	Type   string   `short:"t" help:"Filter by type."`

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

	Deferred bool `name:"deferred" help:"Only issues currently deferred (defer_until in the future)."`
	Overdue  bool `name:"overdue" help:"Only non-closed issues whose defer_until has already passed."`

	LabelPattern string   `name:"label-pattern" help:"Filter by label glob (SQLite GLOB; e.g. 'tech-*')."`
	ExcludeLabel []string `name:"exclude-label" sep:"," help:"Exclude issues that have ANY of these labels."`
	ExcludeType  []string `name:"exclude-type" sep:"," help:"Exclude these issue types."`

	Sort    string `name:"sort" enum:"priority,created,updated,closed,id,title,type," default:"" help:"Sort by field (default: priority, created)."`
	Reverse bool   `short:"r" name:"reverse" help:"Reverse sort order."`

	DescContains     string `name:"desc-contains" help:"Filter by description substring (case-insensitive)."`
	EmptyDescription bool   `name:"empty-description" help:"Only issues with empty or missing description."`
}

func (f listFilterFlags) toFilter() (store.ListFilter, error) {
	out := store.ListFilter{
		Statuses:         expandStatuses(f.Status),
		Assignee:         agentPtr(f.Agent),
		Type:             f.Type,
		Labels:           f.Label,
		LabelsAny:        f.LabelAny,
		NoLabels:         f.NoLabels,
		NoAssignee:       f.NoAssignee,
		TitleContains:    f.TitleContains,
		PriorityMin:      f.PriorityMin,
		PriorityMax:      f.PriorityMax,
		IDs:              f.ID,
		Deferred:         f.Deferred,
		Overdue:          f.Overdue,
		LabelPattern:     f.LabelPattern,
		ExcludeLabels:    f.ExcludeLabel,
		ExcludeTypes:     f.ExcludeType,
		Sort:             f.Sort,
		Reverse:          f.Reverse,
		DescContains:     f.DescContains,
		EmptyDescription: f.EmptyDescription,
	}
	var err error
	if out.CreatedAfter, err = parseDate(f.CreatedAfter); err != nil {
		return out, fmt.Errorf("--created-after: %w", err)
	}
	if out.CreatedBefore, err = parseDate(f.CreatedBefore); err != nil {
		return out, fmt.Errorf("--created-before: %w", err)
	}
	if out.UpdatedAfter, err = parseDate(f.UpdatedAfter); err != nil {
		return out, fmt.Errorf("--updated-after: %w", err)
	}
	if out.UpdatedBefore, err = parseDate(f.UpdatedBefore); err != nil {
		return out, fmt.Errorf("--updated-before: %w", err)
	}
	return out, nil
}

type ListCmd struct {
	listFilterFlags
	Limit         int           `short:"n" default:"50" help:"Limit results (0 = unlimited)."`
	Watch         bool          `short:"w" name:"watch" help:"Keep updating the list when matching issues change. Ctrl+C to exit."`
	WatchInterval time.Duration `name:"interval" default:"1s" help:"Poll interval when --watch is set."`
	Heartbeat     bool          `name:"heartbeat" help:"While watching, register -a <name> as a live agent in 'clu agent ls'. Requires -a. Off by default."`
}

func (c *ListCmd) Run(r *runCtx) error {
	f, err := c.toFilter()
	if err != nil {
		return err
	}
	f.Limit = c.Limit

	render := func(s *store.Store) (string, error) {
		issues, err := s.List(r.ctx, f)
		if err != nil {
			return "", err
		}
		labels, err := loadLabelsFor(r.ctx, s, issues)
		if err != nil {
			return "", err
		}
		blocked, err := loadBlockedFor(r.ctx, s, issues)
		if err != nil {
			return "", err
		}
		// Capture the human/JSON output by swapping stdout to a buffer.
		var buf bytes.Buffer
		sub := *r
		sub.stdout = &buf
		printIssues(&sub, issues, labels, blocked)
		return buf.String(), nil
	}

	return withStore(r, func(s *store.Store) error {
		if c.Watch {
			if r.json {
				return errors.New("--watch is not supported with --json (JSON output is a single document)")
			}
			// Heartbeat is opt-in. -a (lane filter) doubles as the
			// agent identity used to populate active_agents.
			hbName := ""
			var hbCaps []string
			if c.Heartbeat {
				if c.Agent == "" {
					return errors.New("--heartbeat requires -a <name>")
				}
				hbName = c.Agent
				hbCaps = resolveAgent(r.dir, c.Agent)
				cleanup, err := startHeartbeat(s, hbName, hbCaps)
				if err != nil {
					return err
				}
				defer cleanup()
			}
			return watchLoop(r.ctx, r.stdout, c.WatchInterval, func() (string, error) {
				heartbeatTick(s, hbName, hbCaps)
				return render(s)
			})
		}
		out, err := render(s)
		if err != nil {
			return err
		}
		fmt.Fprint(r.stdout, out)
		return nil
	})
}

// expandStatuses translates CLI status tokens into the slice the store expects.
//   - "all" anywhere → nil (no filter)
//   - "active" → "open" + "in_progress"
//   - other tokens → themselves
//
// Duplicates are removed.
func expandStatuses(in []string) []string {
	for _, s := range in {
		if s == "all" {
			return nil
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		switch s {
		case "active":
			for _, v := range []string{"open", "in_progress"} {
				if !seen[v] {
					seen[v] = true
					out = append(out, v)
				}
			}
		default:
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// parseDate accepts the same forms as `clu defer`: YYYY-MM-DD,
// RFC3339, "+Nh/Nd/Nw" relative durations, and "tomorrow". Empty
// string → (nil, nil) — no filter on that dimension. Shared with
// parseWhen so an operator using --created-after sees the same
// parser they get on `clu defer`.
func parseDate(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	t, err := parseWhen(s, time.Now())
	if err != nil {
		return nil, err
	}
	u := t.Unix()
	return &u, nil
}
