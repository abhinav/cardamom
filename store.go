package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rovak/beadsv2/internal/dbq"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// migrations are applied in order; PRAGMA user_version tracks progress.
// Append new migrations, never edit existing ones.
var migrations = []string{
	// v1: initial schema.
	`
    CREATE TABLE IF NOT EXISTS issues (
        id        TEXT PRIMARY KEY,
        title     TEXT NOT NULL,
        type      TEXT NOT NULL DEFAULT 'task',
        status    TEXT NOT NULL DEFAULT 'open',
        priority  INTEGER NOT NULL DEFAULT 2,
        assignee  TEXT,
        created   INTEGER NOT NULL,
        updated   INTEGER NOT NULL,
        closed    INTEGER
    );
    CREATE INDEX IF NOT EXISTS idx_issues_status ON issues(status);

    CREATE TABLE IF NOT EXISTS deps (
        child_id  TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        parent_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        PRIMARY KEY (child_id, parent_id)
    );
    CREATE INDEX IF NOT EXISTS idx_deps_parent ON deps(parent_id);
    `,
	// v2: agent lane.
	`
    ALTER TABLE issues ADD COLUMN agent TEXT;
    CREATE INDEX IF NOT EXISTS idx_issues_agent ON issues(agent);
    `,
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for ; version < len(migrations); version++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[version]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version+1, err)
		}
		// PRAGMA user_version doesn't accept placeholders.
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

type Issue struct {
	ID       string
	Title    string
	Type     string
	Status   string
	Priority int
	Agent    *string
	Assignee *string
	Created  int64
	Updated  int64
	Closed   *int64
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func now() int64 { return time.Now().Unix() }

var (
	ErrNotFound      = errors.New("issue not found")
	ErrAlreadyClosed = errors.New("issue already closed")
	ErrNotClaimable  = errors.New("issue not claimable")
	ErrCycle         = errors.New("dependency would create a cycle")
	ErrSelfDep       = errors.New("issue cannot depend on itself")
)

func isUniqueErr(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code() {
	case sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY,
		sqlite3.SQLITE_CONSTRAINT_UNIQUE:
		return true
	}
	return false
}

// scanIssue reads one issue row from r into i.
func scanIssue(r interface{ Scan(...any) error }, i *Issue) error {
	return r.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Agent, &i.Assignee, &i.Created, &i.Updated, &i.Closed)
}

const issueCols = `id, title, type, status, priority, agent, assignee, created, updated, closed`

// Create inserts a new issue. agent may be nil for an unassigned-lane issue.
func (s *Store) Create(title, typ string, priority int, agent *string) (Issue, error) {
	if title == "" {
		return Issue{}, errors.New("title required")
	}
	if typ == "" {
		typ = "task"
	}
	for tries := 0; tries < 8; tries++ {
		id := NewID()
		t := now()
		_, err := s.db.Exec(
			`INSERT INTO issues (id, title, type, priority, agent, created, updated) VALUES (?,?,?,?,?,?,?)`,
			id, title, typ, priority, agent, t, t,
		)
		if err == nil {
			return s.Get(id)
		}
		if !isUniqueErr(err) {
			return Issue{}, err
		}
	}
	return Issue{}, errors.New("failed to allocate unique id after 8 tries")
}

// Get fetches a single issue by ID.
// This method is wired through sqlc-generated code as a preview of the
// full migration; the rest of Store still uses hand-written SQL.
func (s *Store) Get(id string) (Issue, error) {
	row, err := dbq.New(s.db).GetIssue(context.TODO(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	if err != nil {
		return Issue{}, err
	}
	return issueFromDBQ(row), nil
}

// issueFromDBQ converts a sqlc-generated Issue to the local Issue type.
// Identical field-for-field except Priority (int64 vs int) — this mapping
// goes away if we standardise on the generated type.
func issueFromDBQ(r dbq.Issue) Issue {
	return Issue{
		ID:       r.ID,
		Title:    r.Title,
		Type:     r.Type,
		Status:   r.Status,
		Priority: int(r.Priority),
		Agent:    r.Agent,
		Assignee: r.Assignee,
		Created:  r.Created,
		Updated:  r.Updated,
		Closed:   r.Closed,
	}
}

// List returns issues filtered by status and/or agent.
//   status: empty string = any status; else exact match.
//   agent:  nil = no agent filter; else exact match on agent name.
func (s *Store) List(status string, agent *string) ([]Issue, error) {
	q := `SELECT ` + issueCols + ` FROM issues WHERE 1=1`
	args := []any{}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if agent != nil {
		q += ` AND agent = ?`
		args = append(args, *agent)
	}
	q += ` ORDER BY priority ASC, created ASC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		var i Issue
		if err := scanIssue(rows, &i); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Ready returns open, unclaimed issues with all dependencies closed.
//   agent: nil = unassigned lane (agent IS NULL); else exact match.
func (s *Store) Ready(limit int, agent *string) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
        SELECT ` + issueCols + `
        FROM issues i
        WHERE i.status = 'open' AND i.assignee IS NULL
          AND ` + agentClause(agent) + `
          AND NOT EXISTS (
              SELECT 1 FROM deps d
              JOIN issues p ON p.id = d.parent_id
              WHERE d.child_id = i.id AND p.status != 'closed'
          )
        ORDER BY i.priority ASC, i.created ASC
        LIMIT ?`
	args := agentArgs(agent)
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Issue
	for rows.Next() {
		var i Issue
		if err := scanIssue(rows, &i); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// WaitReady polls Ready until at least one issue is returned, the context is
// cancelled, or an error occurs. Returns whatever issues are ready at that
// instant. Caller controls the poll interval.
func (s *Store) WaitReady(ctx context.Context, limit int, agent *string, interval time.Duration) ([]Issue, error) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		issues, err := s.Ready(limit, agent)
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

// agentClause returns a SQL fragment scoping a query to a lane.
// nil → unassigned lane (agent IS NULL); non-nil → agent = ?.
func agentClause(agent *string) string {
	if agent == nil {
		return `i.agent IS NULL`
	}
	return `i.agent = ?`
}

func agentArgs(agent *string) []any {
	if agent == nil {
		return nil
	}
	return []any{*agent}
}

// Claim atomically assigns the next ready issue in the given lane.
//   agent: nil = unassigned lane (agent IS NULL); else exact match.
// Returns ErrNotFound when nothing is ready in that lane.
func (s *Store) Claim(assignee string, agent *string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	q := `
        UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
        WHERE id = (
            SELECT i.id FROM issues i
            WHERE i.status = 'open' AND i.assignee IS NULL
              AND ` + agentClause(agent) + `
              AND NOT EXISTS (
                  SELECT 1 FROM deps d
                  JOIN issues p ON p.id = d.parent_id
                  WHERE d.child_id = i.id AND p.status != 'closed'
              )
            ORDER BY i.priority ASC, i.created ASC
            LIMIT 1
        )
        RETURNING ` + issueCols
	args := []any{assignee, now()}
	args = append(args, agentArgs(agent)...)
	var i Issue
	err := scanIssue(s.db.QueryRow(q, args...), &i)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

// ClaimByID claims a specific issue if open and unassigned.
func (s *Store) ClaimByID(id, assignee string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	res, err := s.db.Exec(
		`UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
         WHERE id = ? AND status = 'open' AND assignee IS NULL`,
		assignee, now(), id)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Issue{}, fmt.Errorf("%w: %s", ErrNotClaimable, id)
	}
	return s.Get(id)
}

// MarkClosed transitions an open/in-progress issue to closed.
func (s *Store) MarkClosed(id string) (Issue, error) {
	t := now()
	res, err := s.db.Exec(
		`UPDATE issues SET status = 'closed', closed = ?, updated = ? WHERE id = ? AND status != 'closed'`,
		t, t, id)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := s.Get(id); errors.Is(err, ErrNotFound) {
			return Issue{}, ErrNotFound
		}
		return Issue{}, ErrAlreadyClosed
	}
	return s.Get(id)
}

type UpdateFields struct {
	Title    *string
	Type     *string
	Status   *string
	Priority *int
	Assignee **string // outer nil = unchanged; inner nil = clear; else set
	Agent    **string // same semantics as Assignee
}

func (s *Store) Update(id string, f UpdateFields) (Issue, error) {
	if _, err := s.Get(id); err != nil {
		return Issue{}, err
	}
	q := `UPDATE issues SET updated = ?`
	args := []any{now()}
	if f.Title != nil {
		q += `, title = ?`
		args = append(args, *f.Title)
	}
	if f.Type != nil {
		q += `, type = ?`
		args = append(args, *f.Type)
	}
	if f.Status != nil {
		q += `, status = ?`
		args = append(args, *f.Status)
		if *f.Status == "closed" {
			q += `, closed = ?`
			args = append(args, now())
		}
	}
	if f.Priority != nil {
		q += `, priority = ?`
		args = append(args, *f.Priority)
	}
	if f.Assignee != nil {
		q += `, assignee = ?`
		args = append(args, *f.Assignee) // *string; nil clears
	}
	if f.Agent != nil {
		q += `, agent = ?`
		args = append(args, *f.Agent) // *string; nil clears
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	if _, err := s.db.Exec(q, args...); err != nil {
		return Issue{}, err
	}
	return s.Get(id)
}

// AddDep adds a child -> parent dependency in a single transaction.
// Rejects self-deps and cycles. Uses a recursive CTE for cycle detection.
func (s *Store) AddDep(child, parent string) error {
	if child == parent {
		return ErrSelfDep
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := exists(tx, child); err != nil {
		return fmt.Errorf("child: %w", err)
	}
	if err := exists(tx, parent); err != nil {
		return fmt.Errorf("parent: %w", err)
	}

	// Cycle iff parent already (transitively) depends on child.
	var cycle int
	err = tx.QueryRow(`
        WITH RECURSIVE ancestors(id) AS (
            SELECT parent_id FROM deps WHERE child_id = ?
            UNION
            SELECT d.parent_id FROM deps d JOIN ancestors a ON d.child_id = a.id
        )
        SELECT EXISTS(SELECT 1 FROM ancestors WHERE id = ?)`,
		parent, child).Scan(&cycle)
	if err != nil {
		return err
	}
	if cycle == 1 {
		return ErrCycle
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO deps (child_id, parent_id) VALUES (?, ?)`, child, parent); err != nil {
		return err
	}
	return tx.Commit()
}

func exists(tx *sql.Tx, id string) error {
	var n int
	err := tx.QueryRow(`SELECT 1 FROM issues WHERE id = ?`, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) RemoveDep(child, parent string) error {
	_, err := s.db.Exec(`DELETE FROM deps WHERE child_id = ? AND parent_id = ?`, child, parent)
	return err
}

func (s *Store) Deps(id string) (parents, blocks []string, err error) {
	rows, err := s.db.Query(`SELECT parent_id FROM deps WHERE child_id = ? ORDER BY parent_id`, id)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return nil, nil, err
		}
		parents = append(parents, p)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT child_id FROM deps WHERE parent_id = ? ORDER BY child_id`, id)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, nil, err
		}
		blocks = append(blocks, c)
	}
	return parents, blocks, rows.Err()
}
