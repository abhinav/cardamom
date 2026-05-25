package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ---- Models ----

type Issue struct {
	bun.BaseModel `bun:"table:issues,alias:i"`

	ID       string  `bun:"id,pk"`
	Title    string  `bun:"title,notnull"`
	Type     string  `bun:"type,notnull"`
	Status   string  `bun:"status,notnull"`
	Priority int     `bun:"priority,notnull"`
	Agent    *string `bun:"agent"`
	Assignee *string `bun:"assignee"`
	Created  int64   `bun:"created,notnull"`
	Updated  int64   `bun:"updated,notnull"`
	Closed   *int64  `bun:"closed"`
}

type Dep struct {
	bun.BaseModel `bun:"table:deps"`

	ChildID  string `bun:"child_id,pk"`
	ParentID string `bun:"parent_id,pk"`
}

// ---- Migrations (kept manual, independent of Bun) ----

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

// ---- Store ----

type Store struct {
	db *bun.DB
}

func Open(path string) (*Store, error) {
	sqldb, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := migrate(sqldb); err != nil {
		sqldb.Close()
		return nil, err
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())
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

// Create inserts a new issue. agent may be nil for an unassigned-lane issue.
func (s *Store) Create(ctx context.Context, title, typ string, priority int, agent *string) (Issue, error) {
	if title == "" {
		return Issue{}, errors.New("title required")
	}
	if typ == "" {
		typ = "task"
	}
	for tries := 0; tries < 8; tries++ {
		t := now()
		i := Issue{
			ID: NewID(), Title: title, Type: typ, Status: "open",
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

// List returns issues filtered by status and/or agent.
//   status: "" = any status; else exact match.
//   agent:  nil = no agent filter; else exact match on agent name.
func (s *Store) List(ctx context.Context, status string, agent *string) ([]Issue, error) {
	var issues []Issue
	q := s.db.NewSelect().Model(&issues).OrderExpr("priority ASC, created ASC")
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if agent != nil {
		q = q.Where("agent = ?", *agent)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return issues, nil
}

// readyQuery applies the shared WHERE/ORDER for ready-issue selection.
func (s *Store) readyQuery(agent *string) *bun.SelectQuery {
	q := s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.status = 'open' AND i.assignee IS NULL").
		Where("NOT EXISTS (SELECT 1 FROM deps d JOIN issues p ON p.id = d.parent_id WHERE d.child_id = i.id AND p.status != 'closed')").
		OrderExpr("i.priority ASC, i.created ASC")
	if agent == nil {
		q = q.Where("i.agent IS NULL")
	} else {
		q = q.Where("i.agent = ?", *agent)
	}
	return q
}

// Ready returns open, unclaimed issues with all dependencies closed.
//   agent: nil = unassigned lane (agent IS NULL); else exact match.
func (s *Store) Ready(ctx context.Context, limit int, agent *string) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	var issues []Issue
	err := s.readyQuery(agent).
		ColumnExpr("i.*").
		Limit(limit).
		Scan(ctx, &issues)
	if err != nil {
		return nil, err
	}
	return issues, nil
}

// Claim atomically assigns the next ready issue in the given lane.
//   agent: nil = unassigned lane (agent IS NULL); else exact match.
// Returns ErrNotFound when nothing is ready in that lane.
//
// SQLite UPDATE…RETURNING with a subquery WHERE is awkward to express via
// the Bun query builder; raw SQL is clearer here.
func (s *Store) Claim(ctx context.Context, assignee string, agent *string) (Issue, error) {
	if assignee == "" {
		return Issue{}, errors.New("assignee required")
	}
	var (
		laneClause string
		laneArgs   []any
	)
	if agent == nil {
		laneClause = "agent IS NULL"
	} else {
		laneClause = "agent = ?"
		laneArgs = []any{*agent}
	}
	q := `
        UPDATE issues SET assignee = ?, status = 'in_progress', updated = ?
        WHERE id = (
            SELECT id FROM issues
            WHERE status = 'open' AND assignee IS NULL
              AND ` + laneClause + `
              AND NOT EXISTS (
                  SELECT 1 FROM deps d
                  JOIN issues p ON p.id = d.parent_id
                  WHERE d.child_id = issues.id AND p.status != 'closed'
              )
            ORDER BY priority ASC, created ASC
            LIMIT 1
        )
        RETURNING id, title, type, status, priority, agent, assignee, created, updated, closed`
	args := []any{assignee, now()}
	args = append(args, laneArgs...)
	var i Issue
	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&i.ID, &i.Title, &i.Type, &i.Status, &i.Priority,
		&i.Agent, &i.Assignee, &i.Created, &i.Updated, &i.Closed,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Issue{}, ErrNotFound
	}
	return i, err
}

// ClaimByID claims a specific issue if open and unassigned.
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
		return Issue{}, fmt.Errorf("%w: %s", ErrNotClaimable, id)
	}
	return s.Get(ctx, id)
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
	Title    *string
	Type     *string
	Status   *string
	Priority *int
	Assignee **string // outer nil = unchanged; inner nil = clear; else set
	Agent    **string // same semantics as Assignee
}

func (s *Store) Update(ctx context.Context, id string, f UpdateFields) (Issue, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return Issue{}, err
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
	if _, err := q.Exec(ctx); err != nil {
		return Issue{}, err
	}
	return s.Get(ctx, id)
}

// AddDep adds a child -> parent dependency in a single transaction.
// Rejects self-deps and cycles via a recursive CTE.
func (s *Store) AddDep(ctx context.Context, child, parent string) error {
	if child == parent {
		return ErrSelfDep
	}
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := issueExistsTx(ctx, tx, child); err != nil {
			return fmt.Errorf("child: %w", err)
		}
		if err := issueExistsTx(ctx, tx, parent); err != nil {
			return fmt.Errorf("parent: %w", err)
		}
		// Cycle iff parent already (transitively) depends on child.
		var cycle int
		err := tx.QueryRowContext(ctx, `
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
		_, err = tx.NewInsert().
			Model(&Dep{ChildID: child, ParentID: parent}).
			On("CONFLICT DO NOTHING").
			Exec(ctx)
		return err
	})
}

func issueExistsTx(ctx context.Context, tx bun.Tx, id string) error {
	n, err := tx.NewSelect().Model((*Issue)(nil)).Where("id = ?", id).Count(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RemoveDep(ctx context.Context, child, parent string) error {
	_, err := s.db.NewDelete().
		Model((*Dep)(nil)).
		Where("child_id = ? AND parent_id = ?", child, parent).
		Exec(ctx)
	return err
}

// Deps returns the IDs of issues this one depends on (parents) and
// the IDs of issues that depend on it (blocks).
func (s *Store) Deps(ctx context.Context, id string) (parents, blocks []string, err error) {
	if err := s.db.NewSelect().
		Model((*Dep)(nil)).
		Column("parent_id").
		Where("child_id = ?", id).
		OrderExpr("parent_id").
		Scan(ctx, &parents); err != nil {
		return nil, nil, err
	}
	if err := s.db.NewSelect().
		Model((*Dep)(nil)).
		Column("child_id").
		Where("parent_id = ?", id).
		OrderExpr("child_id").
		Scan(ctx, &blocks); err != nil {
		return nil, nil, err
	}
	return parents, blocks, nil
}

// WaitReady polls Ready until at least one issue is returned, the context is
// cancelled, or an error occurs. Caller controls the poll interval.
func (s *Store) WaitReady(ctx context.Context, limit int, agent *string, interval time.Duration) ([]Issue, error) {
	if interval <= 0 {
		interval = 250 * time.Millisecond
	}
	for {
		issues, err := s.Ready(ctx, limit, agent)
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
