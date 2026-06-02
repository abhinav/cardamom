package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Rovak/agents-clu/internal/store"
)

// HistoryCmd prints the audit trail for a single issue, oldest first —
// the timeline of who did what to it.
type HistoryCmd struct {
	ID string `arg:"" help:"Issue ID."`
}

func (c *HistoryCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		evs, err := s.History(r.ctx, c.ID)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(toEventOuts(evs))
		}
		if len(evs) == 0 {
			fmt.Fprintln(r.stdout, "(no events)")
			return nil
		}
		for _, e := range evs {
			printEvent(r, e, false)
		}
		return nil
	})
}

// LogCmd prints the global event stream, newest first, with optional
// filters. The cross-issue companion to `history`.
type LogCmd struct {
	Actor string `name:"actor" help:"Only events by this actor."`
	Kind  string `name:"kind" help:"Only events of this kind (created, claimed, closed, …)."`
	Issue string `name:"issue" help:"Only events for this issue ID."`
	Since string `name:"since" help:"Only events newer than this ago (e.g. 24h, 7d, 2w)."`
	Limit int    `name:"limit" default:"50" help:"Max events to show."`
}

func (c *LogCmd) Run(r *runCtx) error {
	f := store.EventFilter{
		Actor:   c.Actor,
		Kind:    c.Kind,
		IssueID: c.Issue,
		Limit:   c.Limit,
	}
	if c.Since != "" {
		d, err := parseRelDuration(c.Since)
		if err != nil {
			return err
		}
		f.Since = time.Now().Add(-d).Unix()
	}
	return withStore(r, func(s *store.Store) error {
		evs, err := s.EventLog(r.ctx, f)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(toEventOuts(evs))
		}
		if len(evs) == 0 {
			fmt.Fprintln(r.stdout, "(no events)")
			return nil
		}
		for _, e := range evs {
			printEvent(r, e, true)
		}
		return nil
	})
}

// eventOut is the JSON shape: payload is re-nested as a raw object
// instead of the escaped string the column stores.
type eventOut struct {
	ID      int64           `json:"id"`
	IssueID *string         `json:"issue_id,omitempty"`
	Actor   *string         `json:"actor,omitempty"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
	TS      int64           `json:"ts"`
}

func toEventOut(e store.Event) eventOut {
	o := eventOut{ID: e.ID, IssueID: e.IssueID, Actor: e.Actor, Kind: e.Kind, TS: e.TS}
	if e.Payload != nil {
		o.Payload = json.RawMessage(*e.Payload)
	}
	return o
}

func toEventOuts(evs []store.Event) []eventOut {
	out := make([]eventOut, len(evs))
	for i, e := range evs {
		out[i] = toEventOut(e)
	}
	return out
}

// printEvent renders one event line. When withIssue is set (the global
// log) the issue ID is included; `history` omits it since it's implied.
func printEvent(r *runCtx, e store.Event, withIssue bool) {
	ts := time.Unix(e.TS, 0).Format(time.RFC3339)
	actor := "-"
	if e.Actor != nil && *e.Actor != "" {
		actor = *e.Actor
	}
	payload := ""
	if e.Payload != nil {
		payload = "  " + *e.Payload
	}
	if withIssue {
		issue := "-"
		if e.IssueID != nil {
			issue = *e.IssueID
		}
		fmt.Fprintf(r.stdout, "%s  %-12s  %-10s  %-10s%s\n", ts, issue, actor, e.Kind, payload)
	} else {
		fmt.Fprintf(r.stdout, "%s  %-10s  %-10s%s\n", ts, actor, e.Kind, payload)
	}
}
