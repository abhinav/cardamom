package main

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
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
	Assignee sql.NullString
	Created  int64
	Updated  int64
	Closed   sql.NullInt64
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

var ErrNotFound = errors.New("issue not found")

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
		// Retry on PK collision; bubble other errors.
		if !isUniqueErr(err) {
			return Issue{}, err
		}
	}
	return Issue{}, errors.New("failed to allocate unique id after 8 tries")
}

func isUniqueErr(err error) bool {
	return err != nil && contains(err.Error(), "UNIQUE")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func (s *Store) Get(id string) (Issue, error) {
	row := s.db.QueryRow(
		`SELECT id, title, type, status, priority, assignee, created, updated, closed FROM issues WHERE id = ?`, id,
	)
	var i Issue
	err := row.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Assignee, &i.Created, &i.Updated, &i.Closed)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

func (s *Store) List(status string) ([]Issue, error) {
	q := `SELECT id, title, type, status, priority, assignee, created, updated, closed FROM issues`
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
		if err := rows.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Assignee, &i.Created, &i.Updated, &i.Closed); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Ready returns open, unassigned issues whose dependencies are all closed,
// ordered by priority then creation time.
func (s *Store) Ready(limit int) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
        SELECT i.id, i.title, i.type, i.status, i.priority, i.assignee, i.created, i.updated, i.closed
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
		if err := rows.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Assignee, &i.Created, &i.Updated, &i.Closed); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Claim atomically assigns the next ready issue to assignee. Returns ErrNotFound
// if no issue is currently ready.
func (s *Store) Claim(assignee string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	t := now()
	row := s.db.QueryRow(`
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
        RETURNING id, title, type, status, priority, assignee, created, updated, closed`,
		assignee, t)
	var i Issue
	err := row.Scan(&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority, &i.Assignee, &i.Created, &i.Updated, &i.Closed)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

// ClaimByID claims a specific issue if it is open and unassigned.
func (s *Store) ClaimByID(id, assignee string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	t := now()
	res, err := s.db.Exec(
		`UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
         WHERE id = ? AND status = 'open' AND assignee IS NULL`,
		assignee, t, id)
	if err != nil {
		return Issue{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Issue{}, fmt.Errorf("issue %s not claimable (missing, closed, or already assigned)", id)
	}
	return s.Get(id)
}

func (s *Store) Close_(id, _msg string) (Issue, error) {
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
		return Issue{}, fmt.Errorf("issue %s already closed", id)
	}
	return s.Get(id)
}

type UpdateFields struct {
	Title    *string
	Type     *string
	Status   *string
	Priority *int
	Assignee *sql.NullString
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
		args = append(args, *f.Assignee)
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	if _, err := s.db.Exec(q, args...); err != nil {
		return Issue{}, err
	}
	return s.Get(id)
}

func (s *Store) AddDep(child, parent string) error {
	if child == parent {
		return errors.New("issue cannot depend on itself")
	}
	if _, err := s.Get(child); err != nil {
		return fmt.Errorf("child: %w", err)
	}
	if _, err := s.Get(parent); err != nil {
		return fmt.Errorf("parent: %w", err)
	}
	if s.wouldCycle(child, parent) {
		return errors.New("dependency would create a cycle")
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO deps (child_id, parent_id) VALUES (?, ?)`, child, parent)
	return err
}

func (s *Store) RemoveDep(child, parent string) error {
	_, err := s.db.Exec(`DELETE FROM deps WHERE child_id = ? AND parent_id = ?`, child, parent)
	return err
}

// wouldCycle returns true if adding child -> parent would create a cycle.
// That is: parent already (transitively) depends on child.
func (s *Store) wouldCycle(child, parent string) bool {
	seen := map[string]bool{}
	var visit func(node string) bool
	visit = func(node string) bool {
		if node == child {
			return true
		}
		if seen[node] {
			return false
		}
		seen[node] = true
		rows, err := s.db.Query(`SELECT parent_id FROM deps WHERE child_id = ?`, node)
		if err != nil {
			return false
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return false
			}
			if visit(p) {
				return true
			}
		}
		return false
	}
	return visit(parent)
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
