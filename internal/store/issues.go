package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

// Create inserts a new issue. assignee may be nil for an unassigned issue
// (the shared pool). Setting it pre-routes the work without claiming it —
// the issue stays status=open until someone `clu claim`s it.
func (s *Store) Create(ctx context.Context, title, typ string, priority int, assignee *string) (Issue, error) {
	return s.CreateWithLinks(ctx, title, typ, priority, assignee, CreateOpts{})
}

// CreateOpts holds the optional extras CreateWithLinks can wire up
// atomically alongside the new issue: capability labels, parent dep
// edges, description, and notes. All are optional; the zero value
// produces a plain `create "<title>"` equivalent.
type CreateOpts struct {
	Caps        []string // cap:<name> labels for capability routing
	Parents     []string // parent IDs to add child→parent dep edges to
	Description string   // sets the description column
	Notes       string   // sets the notes column
}

// CreateWithLinks inserts an issue and atomically attaches the extras
// in `opts` (cap labels, dep edges, description, notes) in a single
// transaction. Closes the race where bare `Create` + follow-up
// `AddDep` / `SetDescription` leaves the new issue briefly visible
// without its edges or context and a watching claim could grab it.
//
// Parents must already exist; a no-such-parent aborts the whole
// transaction so we never leave a half-linked issue behind.
func (s *Store) CreateWithLinks(ctx context.Context, title, typ string, priority int, assignee *string, opts CreateOpts) (Issue, error) {
	// Trim before the empty check so `clu create "   "` is rejected the
	// same way as `clu create ""`. Bare whitespace stored as a title
	// previously surfaced as a blank line in `list`.
	title = strings.TrimSpace(title)
	if title == "" {
		return Issue{}, fmt.Errorf("%w: title required", ErrInvalid)
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
	for _, c := range opts.Caps {
		if c == "" {
			return Issue{}, fmt.Errorf("%w: capability cannot be empty", ErrInvalid)
		}
	}

	var created Issue
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// INSERT issue with PK-collision retry inside the tx. SQLite's
		// constraint failures don't abort the tx; we can retry the
		// statement with a fresh ID.
		var descPtr, notesPtr *string
		if opts.Description != "" {
			d := opts.Description
			descPtr = &d
		}
		if opts.Notes != "" {
			n := opts.Notes
			notesPtr = &n
		}
		for tries := 0; tries < 8; tries++ {
			t := now()
			i := Issue{
				ID: newID(s.idPrefix), Title: title, Type: typ, Status: "open",
				Priority: priority, Assignee: assignee,
				Created: t, Updated: t,
				Description: descPtr,
				Notes:       notesPtr,
			}
			_, err := tx.NewInsert().Model(&i).Exec(ctx)
			if err == nil {
				created = i
				break
			}
			if !isUniqueErr(err) {
				return err
			}
		}
		if created.ID == "" {
			return errors.New("failed to allocate unique id after 8 tries")
		}
		// Cap labels.
		if len(opts.Caps) > 0 {
			rows := make([]IssueLabel, len(opts.Caps))
			for j, cap := range opts.Caps {
				rows[j] = IssueLabel{IssueID: created.ID, Label: "cap:" + cap}
			}
			if _, err := tx.NewInsert().Model(&rows).On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
				return err
			}
		}
		// Dep edges. Verify each parent exists; no cycle check needed
		// since the new issue can't yet have descendants. The whole tx
		// rolls back if any parent is missing, so we never publish a
		// half-linked issue.
		for _, parent := range opts.Parents {
			if parent == created.ID {
				return ErrSelfDep // defensive; shouldn't be reachable
			}
			if err := issueExistsTx(ctx, tx, parent); err != nil {
				return fmt.Errorf("dep %s: %w", parent, err)
			}
			if _, err := tx.NewInsert().
				Model(&Dep{ChildID: created.ID, ParentID: parent}).
				On("CONFLICT DO NOTHING").Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Issue{}, err
	}
	changed := map[string]any{"title": created.Title, "type": created.Type, "priority": created.Priority}
	if created.Assignee != nil {
		changed["assignee"] = *created.Assignee
	}
	// Record the atomically-attached extras so history reflects what was
	// actually created (no separate labeled/dep_added events follow a
	// CreateWithLinks). Description/notes are recorded as presence flags,
	// not text, matching the changed-fields-only payload convention.
	if len(opts.Caps) > 0 {
		caps := make([]string, len(opts.Caps))
		for i, c := range opts.Caps {
			caps[i] = "cap:" + c
		}
		changed["labels"] = caps
	}
	if len(opts.Parents) > 0 {
		changed["depends_on"] = opts.Parents
	}
	if created.Description != nil {
		changed["description"] = true
	}
	if created.Notes != nil {
		changed["notes"] = true
	}
	s.recordEvent(ctx, created.ID, "created", changed)
	return created, nil
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
		Set("started_at = NULL"). // reopened → no longer in progress
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
	s.recordEvent(ctx, id, "reopened", map[string]any{"status": "open"})
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
            SET status = 'cancelled', closed = ?, updated = ?, defer_until = NULL
            WHERE id IN (`+placeholders(len(ids))+`)`,
			setArgs...)
		if err != nil {
			return err
		}
		return tx.NewSelect().Model(&changed).
			Where("id IN (?)", bun.List(ids)).
			OrderExpr("id ASC").
			Scan(ctx)
	})
	if err == nil {
		for _, c := range changed {
			s.recordEvent(ctx, c.ID, "cancelled", map[string]any{"status": "cancelled"})
		}
	}
	return changed, err
}

// MarkClosed transitions an open/in-progress issue to closed.
// Clears defer_until — once an issue is terminal, the wait window
// is moot, and the doctor's "Closed+deferred" check otherwise flags
// every previously-deferred issue we close.
func (s *Store) MarkClosed(ctx context.Context, id string) (Issue, error) {
	t := now()
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("status = 'closed'").
		Set("closed = ?", t).
		Set("updated = ?", t).
		Set("defer_until = NULL").
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
	s.recordEvent(ctx, id, "closed", map[string]any{"status": "closed"})
	s.closeSatisfiedMilestones(ctx, id)
	return s.Get(ctx, id)
}

type UpdateFields struct {
	Title       *string
	Type        *string
	Status      *string
	Priority    *int
	Assignee    **string // outer nil = unchanged; inner nil = clear; else set
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
		// in_progress without an assignee is a stuck row waiting to
		// happen — doctor's "Stuck in_progress" check would fire on
		// it after the threshold. claim is the documented path that
		// sets assignee + status atomically. Refuse the bare update.
		if *f.Status == "in_progress" {
			cur, gerr := s.Get(ctx, id)
			if gerr != nil {
				return Issue{}, gerr
			}
			resultingAssignee := cur.Assignee
			if f.Assignee != nil {
				resultingAssignee = *f.Assignee
			}
			if resultingAssignee == nil {
				return Issue{}, fmt.Errorf("%w: in_progress requires an assignee — use `clu claim` to take an issue, or pass --assignee", ErrInvalid)
			}
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
		// Keep `closed` (the timestamp) in lock-step with the status.
		// MarkClosed/Cancel set it on the way in; the inverse — clearing
		// when status transitions out of closed/cancelled — has to be
		// explicit here too, otherwise `show` displays a stale Closed:
		// timestamp on an issue whose status is open/in_progress.
		switch *f.Status {
		case "closed", "cancelled":
			// Terminal transitions clear defer_until too (doctor's
			// Closed+deferred check otherwise fires on any previously
			// deferred row we close). started_at is preserved → closed -
			// started_at is the cycle time.
			q = q.Set("closed = ?", now()).Set("defer_until = NULL")
		case "in_progress":
			// Entering in_progress via update (with an assignee) stamps the
			// start, same as claim.
			q = q.Set("closed = NULL").Set("started_at = ?", now())
		default: // open
			q = q.Set("closed = NULL").Set("started_at = NULL")
		}
	}
	if f.Priority != nil {
		q = q.Set("priority = ?", *f.Priority)
	}
	if f.Assignee != nil {
		q = q.Set("assignee = ?", *f.Assignee)
	}
	if f.Description != nil {
		q = q.Set("description = ?", *f.Description)
	}
	if _, err := q.Exec(ctx); err != nil {
		return Issue{}, err
	}
	changed := map[string]any{}
	if f.Title != nil {
		changed["title"] = *f.Title
	}
	if f.Type != nil {
		changed["type"] = *f.Type
	}
	if f.Status != nil {
		changed["status"] = *f.Status
	}
	if f.Priority != nil {
		changed["priority"] = *f.Priority
	}
	if f.Assignee != nil {
		changed["assignee"] = *f.Assignee // *string: nil means cleared
	}
	if f.Description != nil {
		changed["description"] = *f.Description != nil
	}
	// When the status changes, emit the semantic kind matching the
	// resulting state (closed/cancelled/reopened) so `log --kind closed`
	// finds transitions made via `update`, not just via `close`/`cancel`.
	// The full changed-field set is preserved in the payload either way.
	kind := "updated"
	if f.Status != nil {
		switch *f.Status {
		case "closed":
			kind = "closed"
		case "cancelled":
			kind = "cancelled"
		case "open":
			kind = "reopened"
		}
	}
	s.recordEvent(ctx, id, kind, changed)
	// A close via `update --status closed` can satisfy dependent milestones,
	// same as MarkClosed. (Cancel does not — a cancelled dep never satisfies
	// a milestone.)
	if kind == "closed" {
		s.closeSatisfiedMilestones(ctx, id)
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
	if until != nil {
		s.recordEvent(ctx, id, "deferred", map[string]any{"defer_until": *until})
	} else {
		s.recordEvent(ctx, id, "undeferred", nil)
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
	s.recordEvent(ctx, id, "notes", map[string]any{"cleared": val == nil})
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
	return UpsertIssueTx(ctx, s.db, i)
}

// DeleteIssue hard-deletes an issue and, via ON DELETE CASCADE
// (foreign_keys is enabled at Open), its dep edges, labels, and
// comments. Returns ErrNotFound if no such issue exists.
//
// This is the destructive counterpart to Cancel (which only flips
// status). It exists for the git-ref sync path: applying an incoming
// tombstone means physically removing the record so it doesn't get
// re-exported and resurrected on the next round. Deliberately not wired
// to a user-facing command — `cancel` remains the recoverable default.
func (s *Store) DeleteIssue(ctx context.Context, id string) error {
	if err := DeleteIssueTx(ctx, s.db, id); err != nil {
		return err
	}
	s.recordEvent(ctx, id, "deleted", nil)
	return nil
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
