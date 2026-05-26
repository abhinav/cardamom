package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

// laneMatchSQL builds the WHERE-clause fragment that selects issues for a
// given agent identity. Returns the fragment (suitable as one Where()
// argument; uses ? placeholders) plus the args to pass.
//
// Matching matrix:
//
//	agent  caps    matches
//	nil    []      i.agent IS NULL AND NOT EXISTS(label LIKE 'cap:%')
//	nil    [...]   i.agent IS NULL AND EXISTS(label IN (...))
//	"X"    []      i.agent = "X"
//	"X"    [...]   i.agent = "X" OR (i.agent IS NULL AND EXISTS(label IN (...)))
//
// Capability-labeled issues are excluded from the plain default claim
// so they don't get grabbed by an agent that doesn't advertise the
// required capability.
func laneMatchSQL(agent *string, caps []string) (string, []any) {
	capList := make([]any, 0, len(caps))
	for _, c := range caps {
		capList = append(capList, "cap:"+c)
	}
	hasCaps := len(caps) > 0
	if agent == nil {
		if !hasCaps {
			return "i.agent IS NULL AND NOT EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = i.id AND label LIKE 'cap:%')", nil
		}
		ph := placeholders(len(caps))
		return "i.agent IS NULL AND EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = i.id AND label IN (" + ph + "))", capList
	}
	if !hasCaps {
		return "i.agent = ?", []any{*agent}
	}
	ph := placeholders(len(caps))
	args := []any{*agent}
	args = append(args, capList...)
	return "(i.agent = ? OR (i.agent IS NULL AND EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = i.id AND label IN (" + ph + "))))", args
}

// readyQuery applies the shared WHERE/ORDER for ready-issue selection.
// Excludes issues whose defer_until is still in the future. caps may be
// nil — see laneMatchSQL for the matching matrix.
func (s *Store) readyQuery(agent *string, caps []string) *bun.SelectQuery {
	q := s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.status = 'open' AND i.assignee IS NULL").
		Where("(i.defer_until IS NULL OR i.defer_until <= ?)", now()).
		Where("NOT EXISTS (SELECT 1 FROM deps d JOIN issues p ON p.id = d.parent_id WHERE d.child_id = i.id AND p.status != 'closed')").
		OrderExpr("i.priority ASC, i.created ASC")
	frag, args := laneMatchSQL(agent, caps)
	q = q.Where(frag, args...)
	return q
}

// Ready returns open, unclaimed issues with all dependencies closed.
//
//	agent: nil = unassigned lane (agent IS NULL); else exact match.
//	caps:  optional capability advertisement; cap:X-labeled issues
//	       in the unassigned lane match when X is in caps.
func (s *Store) Ready(ctx context.Context, limit int, agent *string, caps []string) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	var issues []Issue
	err := s.readyQuery(agent, caps).
		ColumnExpr("i.*").
		Limit(limit).
		Scan(ctx, &issues)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// Claim atomically assigns the next ready issue in the given lane.
//
//	agent: nil = unassigned lane (agent IS NULL); else exact match.
//	caps:  optional capabilities the caller advertises; cap:X-labeled
//	       issues in the unassigned lane are reachable when X is in caps.
//
// Returns ErrNotFound when nothing is ready in that identity's reach.
//
// SQLite UPDATE…RETURNING with a subquery WHERE is awkward to express via
// the Bun query builder; raw SQL is clearer here. The lane+capability
// fragment is shared with readyQuery via laneMatchSQL — keep them in sync.
func (s *Store) Claim(ctx context.Context, assignee string, agent *string, caps []string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	// laneMatchSQL returns SQL using "i.agent"; the raw-SQL subquery
	// here references unqualified "agent" / "issue_id". Rewrite the
	// table-qualified references.
	frag, laneArgs := laneMatchSQL(agent, caps)
	frag = strings.ReplaceAll(frag, "i.agent", "agent")
	frag = strings.ReplaceAll(frag, "i.id", "issues.id")

	q := `
        UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
        WHERE id = (
            SELECT id FROM issues
            WHERE status = 'open' AND assignee IS NULL
              AND ` + frag + `
              AND (defer_until IS NULL OR defer_until <= ?)
              AND NOT EXISTS (
                  SELECT 1 FROM deps d
                  JOIN issues p ON p.id = d.parent_id
                  WHERE d.child_id = issues.id AND p.status != 'closed'
              )
            ORDER BY priority ASC, created ASC
            LIMIT 1
        )
        RETURNING id, title, type, status, priority, agent, assignee, created, updated, closed, defer_until, description, notes`
	t := now()
	args := []any{assignee, t}
	args = append(args, laneArgs...)
	args = append(args, t)
	var i Issue
	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority,
		&i.Agent, &i.Assignee, &i.Created, &i.Updated, &i.Closed, &i.DeferUntil, &i.Description, &i.Notes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

// ClaimByID claims a specific issue if open and unassigned.
// Distinguishes "missing issue" (ErrNotFound) from "exists but already
// claimed / not open" (ErrNotClaimable) so error messages match the
// failure mode.
func (s *Store) ClaimByID(ctx context.Context, id, assignee string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("assignee = ?", assignee).
		Set("status = 'in_progress'").
		Set("updated = ?", now()).
		Where("id = ? AND status = 'open' AND assignee IS NULL", id).
		Exec(ctx)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.Get(ctx, id); errors.Is(err, ErrNotFound) {
			return Issue{}, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return Issue{}, fmt.Errorf("%w: %s", ErrNotClaimable, id)
	}
	return s.Get(ctx, id)
}

// WaitReady polls Ready until at least one issue is returned, the context is
// cancelled, or an error occurs. Caller controls the poll interval.
func (s *Store) WaitReady(ctx context.Context, limit int, agent *string, caps []string, interval time.Duration) ([]Issue, error) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		issues, err := s.Ready(ctx, limit, agent, caps)
		if err != nil {
			return nil, err
		}
		if len(issues) > 0 {
			return issues, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
