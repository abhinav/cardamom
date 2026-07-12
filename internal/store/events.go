package store

import (
	"context"
	"encoding/json"
)

// recordEvent appends one row to the audit log. It is best-effort: a
// failure to record never fails the originating write (the event is
// recorded after the write has already succeeded, so atomicity isn't
// guaranteed — a crash in the gap leaves a missing entry, acceptable for
// a local audit log). `changed` holds only the fields that changed; nil
// or empty → a NULL payload.
//
// Actor comes from the Store (set per CLI invocation via SetActor).
func (s *Store) recordEvent(ctx context.Context, issueID, kind string, changed map[string]any) {
	ev := Event{Kind: kind, TS: now()}
	if issueID != "" {
		ev.IssueID = &issueID
	}
	if s.actor != "" {
		a := s.actor
		ev.Actor = &a
	}
	if len(changed) > 0 {
		if b, err := json.Marshal(changed); err == nil {
			p := string(b)
			ev.Payload = &p
		}
	}
	_, _ = s.db.NewInsert().Model(&ev).Exec(ctx)
}

// RecordLabeled records a "labeled" audit event for issueID. Exported
// for callers (label propagate) that add labels through a transaction
// helper instead of AddLabels and so need to log the change themselves.
// No-op when added is empty.
func (s *Store) RecordLabeled(ctx context.Context, issueID string, added []string) {
	if len(added) == 0 {
		return
	}
	s.recordEvent(ctx, issueID, "labeled", map[string]any{"added": added})
}

// EventFilter narrows EventLog. Zero value matches everything.
type EventFilter struct {
	IssueID string // exact issue scope; empty = any
	Actor   string // exact actor; empty = any
	Kind    string // exact kind; empty = any
	Since   int64  // ts >= Since; 0 = no lower bound
	Until   int64  // ts <= Until; 0 = no upper bound
	Limit   int    // max rows; <= 0 = default 50
}

// History returns every event for one issue, oldest first. The issue
// need not still exist — events outlive their issues.
func (s *Store) History(ctx context.Context, issueID string) ([]Event, error) {
	var evs []Event
	err := s.db.NewSelect().
		Model(&evs).
		Where("issue_id = ?", issueID).
		OrderExpr("ts ASC, id ASC").
		Scan(ctx)
	return evs, err
}

// EventLog returns events matching the filter, newest first (most recent
// activity is what a coordinator wants to see at the top).
func (s *Store) EventLog(ctx context.Context, f EventFilter) ([]Event, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	q := s.db.NewSelect().Model((*Event)(nil)).
		OrderExpr("ts DESC, id DESC").
		Limit(limit)
	if f.IssueID != "" {
		q = q.Where("issue_id = ?", f.IssueID)
	}
	if f.Actor != "" {
		q = q.Where("actor = ?", f.Actor)
	}
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Since != 0 {
		q = q.Where("ts >= ?", f.Since)
	}
	if f.Until != 0 {
		q = q.Where("ts <= ?", f.Until)
	}
	var evs []Event
	if err := q.Scan(ctx, &evs); err != nil {
		return nil, err
	}
	return evs, nil
}

// GetEvent fetches one event by its row id — the handle for drilling
// into a specific change.
func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	var ev Event
	err := s.db.NewSelect().Model(&ev).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return Event{}, err
	}
	return ev, nil
}
