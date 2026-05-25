package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const schema = `
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
`

type Issue struct {
	ID       string
	Title    string
	Type     string
	Status   string
	Priority int
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
	if _, err := db.Exec(schema); err != nil {
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
	return r.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Assignee, &i.Created, &i.Updated, &i.Closed)
}

const issueCols = `id, title, type, status, priority, assignee, created, updated, closed`

func (s *Store) Create(title, typ string, priority int) (Issue, error) {
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
			`INSERT INTO issues (id, title, type, priority, created, updated) VALUES (?,?,?,?,?,?)`,
			id, title, typ, priority, t, t,
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

func (s *Store) Get(id string) (Issue, error) {
	var i Issue
	err := scanIssue(s.db.QueryRow(`SELECT `+issueCols+` FROM issues WHERE id = ?`, id), &i)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

func (s *Store) List(status string) ([]Issue, error) {
	q := `SELECT ` + issueCols + ` FROM issues`
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
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

// Ready returns open, unassigned issues with all dependencies closed.
func (s *Store) Ready(limit int) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
        SELECT `+issueCols+`
        FROM issues i
        WHERE i.status = 'open' AND i.assignee IS NULL
          AND NOT EXISTS (
              SELECT 1 FROM deps d
              JOIN issues p ON p.id = d.parent_id
              WHERE d.child_id = i.id AND p.status != 'closed'
          )
        ORDER BY i.priority ASC, i.created ASC
        LIMIT ?`, limit)
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

// Claim atomically assigns the next ready issue. Returns ErrNotFound when none.
func (s *Store) Claim(assignee string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	var i Issue
	err := scanIssue(s.db.QueryRow(`
        UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
        WHERE id = (
            SELECT i.id FROM issues i
            WHERE i.status = 'open' AND i.assignee IS NULL
              AND NOT EXISTS (
                  SELECT 1 FROM deps d
                  JOIN issues p ON p.id = d.parent_id
                  WHERE d.child_id = i.id AND p.status != 'closed'
              )
            ORDER BY i.priority ASC, i.created ASC
            LIMIT 1
        )
        RETURNING `+issueCols, assignee, now()), &i)
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
