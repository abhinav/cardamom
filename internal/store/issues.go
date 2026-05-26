package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/uptrace/bun"
)

// Create inserts a new issue. agent may be nil for an unassigned-lane issue.
func (s *Store) Create(ctx context.Context, title, typ string, priority int, agent *string) (Issue, error) {
	if title == "" {
		return Issue{}, errors.New("title required")
	}
	if typ == "" {
		typ = "task"
	}
	if err := ValidateType(typ); err != nil {
		return Issue{}, err
	}
	if err := ValidatePriority(priority); err != nil {
		return Issue{}, err
	}
	for tries := 0; tries < 8; tries++ {
		t := now()
		i := Issue{
			ID: newID(s.idPrefix), Title: title, Type: typ, Status: "open",
			Priority: priority, Agent: agent,
			Created: t, Updated: t,
		}
		_, err := s.db.NewInsert().Model(&i).Exec(ctx)
		if err == nil {
			return i, nil
		}
		if !isUniqueErr(err) {
			return Issue{}, err
		}
	}
	return Issue{}, errors.New("failed to allocate unique id after 8 tries")
}

func (s *Store) Get(ctx context.Context, id string) (Issue, error) {
	var i Issue
	err := s.db.NewSelect().Model(&i).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

// Reopen transitions a closed issue back to open and clears the closed
// timestamp. Symmetric with MarkClosed; also accepts cancelled→open so
// `clu cancel` is reversible via `clu reopen`.
func (s *Store) Reopen(ctx context.Context, id string) (Issue, error) {
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("status = 'open'").
		Set("closed = NULL").
		Set("updated = ?", now()).
		Where("id = ? AND status IN ('closed', 'cancelled')", id).
		Exec(ctx)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.Get(ctx, id); errors.Is(err, ErrNotFound) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, ErrAlreadyOpen
	}
	return s.Get(ctx, id)
}

// Cancel marks the given issue and all transitive descendants (issues
// that depend on it, directly or transitively) as cancelled. Already-
// terminal issues (closed or cancelled) are skipped — they're not
// re-touched. Returns the IDs that were actually cancelled.
//
// Cascade is the whole point: cancelling A means "we're not doing A,
// or anything that needed A done first." If you want to cancel only
// the target without cascading, use `update --status cancelled <id>`.
func (s *Store) Cancel(ctx context.Context, roots []string) ([]Issue, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	// Verify roots exist first so we error early on typos.
	for _, id := range roots {
		if _, err := s.Get(ctx, id); err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
	}
	// Find descendants via recursive CTE, then UPDATE in one statement.
	// SQLite UPDATE returns affected rows but not the IDs, so we run
	// the CTE twice (once for the IN list, once for the result) inside
	// a transaction.
	t := now()
	var changed []Issue
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		ph := placeholders(len(roots))
		args := make([]any, 0, len(roots))
		for _, r := range roots {
			args = append(args, r)
		}
		rows, err := tx.QueryContext(ctx, `
            WITH RECURSIVE closure(id) AS (
                SELECT id FROM issues WHERE id IN (`+ph+`)
                UNION
                SELECT d.child_id FROM deps d JOIN closure c ON d.parent_id = c.id
            )
            SELECT i.id FROM issues i
            JOIN closure ON closure.id = i.id
            WHERE i.status NOT IN ('closed', 'cancelled')`,
			args...)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if len(ids) == 0 {
			return nil
		}
		idArgs := make([]any, 0, len(ids))
		for _, id := range ids {
			idArgs = append(idArgs, id)
		}
		setArgs := []any{t, t}
		setArgs = append(setArgs, idArgs...)
		_, err = tx.ExecContext(ctx, `
            UPDATE issues
            SET status = 'cancelled', closed = ?, updated = ?
            WHERE id IN (`+placeholders(len(ids))+`)`,
			setArgs...)
		if err != nil {
			return err
		}
		return tx.NewSelect().Model(&changed).
			Where("id IN (?)", bun.In(ids)).
			OrderExpr("id ASC").
			Scan(ctx)
	})
	return changed, err
}

// MarkClosed transitions an open/in-progress issue to closed.
func (s *Store) MarkClosed(ctx context.Context, id string) (Issue, error) {
	t := now()
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("status = 'closed'").
		Set("closed = ?", t).
		Set("updated = ?", t).
		Where("id = ? AND status != 'closed'", id).
		Exec(ctx)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.Get(ctx, id); errors.Is(err, ErrNotFound) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, ErrAlreadyClosed
	}
	return s.Get(ctx, id)
}

type UpdateFields struct {
	Title       *string
	Type        *string
	Status      *string
	Priority    *int
	Assignee    **string // outer nil = unchanged; inner nil = clear; else set
	Agent       **string // same semantics as Assignee
	Description **string // same semantics as Assignee
}

func (s *Store) Update(ctx context.Context, id string, f UpdateFields) (Issue, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Issue{}, err
	}
	if f.Status != nil {
		if err := ValidateStatus(*f.Status); err != nil {
			return Issue{}, err
		}
	}
	if f.Type != nil {
		if err := ValidateType(*f.Type); err != nil {
			return Issue{}, err
		}
	}
	if f.Priority != nil {
		if err := ValidatePriority(*f.Priority); err != nil {
			return Issue{}, err
		}
	}
	q := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("updated = ?", now()).
		Where("id = ?", id)
	if f.Title != nil {
		q = q.Set("title = ?", *f.Title)
	}
	if f.Type != nil {
		q = q.Set("type = ?", *f.Type)
	}
	if f.Status != nil {
		q = q.Set("status = ?", *f.Status)
		if *f.Status == "closed" {
			q = q.Set("closed = ?", now())
		}
	}
	if f.Priority != nil {
		q = q.Set("priority = ?", *f.Priority)
	}
	if f.Assignee != nil {
		q = q.Set("assignee = ?", *f.Assignee)
	}
	if f.Agent != nil {
		q = q.Set("agent = ?", *f.Agent)
	}
	if f.Description != nil {
		q = q.Set("description = ?", *f.Description)
	}
	if _, err := q.Exec(ctx); err != nil {
		return Issue{}, err
	}
	return s.Get(ctx, id)
}

// SetDefer sets or clears an issue's defer_until. Pass nil to clear.
func (s *Store) SetDefer(ctx context.Context, id string, until *int64) (Issue, error) {
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("defer_until = ?", until).
		Set("updated = ?", now()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Issue{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

// SetNotes replaces an issue's notes. Pass an empty string to clear.
func (s *Store) SetNotes(ctx context.Context, id, text string) (Issue, error) {
	var val *string
	if text != "" {
		v := text
		val = &v
	}
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("notes = ?", val).
		Set("updated = ?", now()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return Issue{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Issue{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

// AppendNote appends text to an issue's notes, separated by a blank line.
// If notes is currently empty, it just sets it.
func (s *Store) AppendNote(ctx context.Context, id, text string) (Issue, error) {
	if text == "" {
		return s.Get(ctx, id)
	}
	cur, err := s.Get(ctx, id)
	if err != nil {
		return Issue{}, err
	}
	combined := text
	if cur.Notes != nil && *cur.Notes != "" {
		combined = *cur.Notes + "\n\n" + text
	}
	return s.SetNotes(ctx, id, combined)
}

// UpsertIssue inserts an issue with an explicit ID, or updates every
// field if the ID already exists. Used by import; bypasses the random-ID
// generation in Create.
func (s *Store) UpsertIssue(ctx context.Context, i Issue) error {
	_, err := s.db.NewInsert().Model(&i).
		On("CONFLICT (id) DO UPDATE").
		Set("title = EXCLUDED.title").
		Set("type = EXCLUDED.type").
		Set("status = EXCLUDED.status").
		Set("priority = EXCLUDED.priority").
		Set("agent = EXCLUDED.agent").
		Set("assignee = EXCLUDED.assignee").
		Set("created = EXCLUDED.created").
		Set("updated = EXCLUDED.updated").
		Set("closed = EXCLUDED.closed").
		Set("defer_until = EXCLUDED.defer_until").
		Set("description = EXCLUDED.description").
		Set("notes = EXCLUDED.notes").
		Exec(ctx)
	return err
}

// exists reports ErrNotFound if no issue with id exists.
func (s *Store) exists(ctx context.Context, id string) error {
	n, err := s.db.NewSelect().Model((*Issue)(nil)).Where("id = ?", id).Count(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
