package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"modernc.org/sqlite"
	_ "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ---- Models ----

type Issue struct {
	bun.BaseModel `bun:"table:issues,alias:i" json:"-"`

	ID       string  `bun:"id,pk" json:"id"`
	Title    string  `bun:"title,notnull" json:"title"`
	Type     string  `bun:"type,notnull" json:"type"`
	Status   string  `bun:"status,notnull" json:"status"`
	Priority int     `bun:"priority,notnull" json:"priority"`
	Agent    *string `bun:"agent" json:"agent,omitempty"`
	Assignee *string `bun:"assignee" json:"assignee,omitempty"`
	Created     int64   `bun:"created,notnull" json:"created"`
	Updated     int64   `bun:"updated,notnull" json:"updated"`
	Closed      *int64  `bun:"closed" json:"closed,omitempty"`
	DeferUntil  *int64  `bun:"defer_until" json:"defer_until,omitempty"`
	Description *string `bun:"description" json:"description,omitempty"`
	Notes       *string `bun:"notes" json:"notes,omitempty"`
}

type Dep struct {
	bun.BaseModel `bun:"table:deps"`

	ChildID  string `bun:"child_id,pk"`
	ParentID string `bun:"parent_id,pk"`
}

type IssueLabel struct {
	bun.BaseModel `bun:"table:issue_labels"`

	IssueID string `bun:"issue_id,pk"`
	Label   string `bun:"label,pk"`
}

type Comment struct {
	bun.BaseModel `bun:"table:comments,alias:c" json:"-"`

	ID      int64  `bun:"id,pk,autoincrement" json:"id"`
	IssueID string `bun:"issue_id,notnull" json:"issue_id"`
	Author  string `bun:"author,notnull" json:"author"`
	Body    string `bun:"body,notnull" json:"body"`
	Created int64  `bun:"created,notnull" json:"created"`
}

// KV is one entry in the generic key-value store. Independent of issues —
// use for feature flags, config snippets, persistent agent scratch data, etc.
type KV struct {
	bun.BaseModel `bun:"table:kv" json:"-"`

	Key   string `bun:"key,pk" json:"key"`
	Value string `bun:"value,notnull" json:"value"`
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
	// v3: labels.
	`
    CREATE TABLE IF NOT EXISTS issue_labels (
        issue_id TEXT NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        label    TEXT NOT NULL,
        PRIMARY KEY (issue_id, label)
    );
    CREATE INDEX IF NOT EXISTS idx_issue_labels_label ON issue_labels(label);
    `,
	// v4: defer_until.
	`
    ALTER TABLE issues ADD COLUMN defer_until INTEGER;
    CREATE INDEX IF NOT EXISTS idx_issues_defer_until ON issues(defer_until);
    `,
	// v5: description.
	`
    ALTER TABLE issues ADD COLUMN description TEXT;
    `,
	// v6: notes.
	`
    ALTER TABLE issues ADD COLUMN notes TEXT;
    `,
	// v7: comments.
	`
    CREATE TABLE IF NOT EXISTS comments (
        id       INTEGER PRIMARY KEY AUTOINCREMENT,
        issue_id TEXT    NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
        author   TEXT    NOT NULL,
        body     TEXT    NOT NULL,
        created  INTEGER NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_comments_issue ON comments(issue_id, id);
    `,
	// v8: kv (generic key-value store).
	`
    CREATE TABLE IF NOT EXISTS kv (
        key   TEXT PRIMARY KEY,
        value TEXT NOT NULL
    );
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

// ValidStatuses are the statuses an issue may take. Source of truth for
// CLI enum validation and `cli statuses`.
var ValidStatuses = []string{"open", "in_progress", "closed"}

// ValidTypes are the canonical issue types. Source of truth for
// `cli types`. The schema does not enforce these — any string is
// allowed at the DB level — but the CLI uses this list for help text
// and discoverability.
var ValidTypes = []string{"task", "bug", "feature", "epic", "chore", "decision"}

// Valid priority range, inclusive. 0 = highest, 4 = lowest. Five
// buckets keeps the urgency hierarchy meaningful without explicit
// label proliferation.
const (
	MinPriority = 0
	MaxPriority = 4
)

// ValidateStatus returns nil if s is in ValidStatuses, else an error.
func ValidateStatus(s string) error {
	for _, v := range ValidStatuses {
		if v == s {
			return nil
		}
	}
	return fmt.Errorf("invalid status %q (valid: %s)", s, strings.Join(ValidStatuses, ", "))
}

// ValidateType returns nil if t is in ValidTypes, else an error.
func ValidateType(t string) error {
	for _, v := range ValidTypes {
		if v == t {
			return nil
		}
	}
	return fmt.Errorf("invalid type %q (valid: %s)", t, strings.Join(ValidTypes, ", "))
}

// ValidatePriority returns nil if p is within [MinPriority, MaxPriority].
func ValidatePriority(p int) error {
	if p < MinPriority || p > MaxPriority {
		return fmt.Errorf("invalid priority %d (must be %d..%d)", p, MinPriority, MaxPriority)
	}
	return nil
}

// SchemaVersion is the number of migrations applied to a fresh DB.
// Equal to PRAGMA user_version after Open() completes.
func SchemaVersion() int { return len(migrations) }

var (
	ErrNotFound      = errors.New("issue not found")
	ErrAlreadyClosed = errors.New("issue already closed")
	ErrAlreadyOpen   = errors.New("issue already open")
	ErrNotClaimable  = errors.New("issue not claimable")
	ErrCycle         = errors.New("dependency would create a cycle")
	ErrSelfDep       = errors.New("issue cannot depend on itself")
	ErrKVNotFound    = errors.New("key not found")
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
	if err := ValidateType(typ); err != nil {
		return Issue{}, err
	}
	if err := ValidatePriority(priority); err != nil {
		return Issue{}, err
	}
	for tries := 0; tries < 8; tries++ {
		t := now()
		i := Issue{
			ID: newID(), Title: title, Type: typ, Status: "open",
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

// ListFilter scopes the result set for Store.List. Zero/nil values mean
// "no filter on this dimension".
type ListFilter struct {
	Statuses      []string // any-of match (e.g. {"open","in_progress"}). nil/empty = no filter.
	Agent         *string  // exact match on agent lane (nil = no filter)
	Type          string   // exact match (e.g. "bug")
	Labels        []string // AND: issue must have ALL of these labels
	LabelsAny     []string // OR: issue must have AT LEAST ONE
	NoLabels      bool     // only issues with no labels at all
	NoAssignee    bool     // assignee IS NULL
	TitleContains string   // case-insensitive substring match
	PriorityMin   *int     // inclusive
	PriorityMax   *int     // inclusive
	CreatedAfter  *int64   // unix seconds, inclusive
	CreatedBefore *int64
	UpdatedAfter  *int64
	UpdatedBefore *int64
	IDs           []string // exact match against any of these IDs
	Deferred      bool     // only issues with defer_until > now (still waiting)
	Overdue       bool     // only non-closed issues with defer_until <= now (ready to be picked up but still tagged)

	LabelPattern  string   // SQLite GLOB (e.g. "tech-*") — issue must have at least one matching label
	ExcludeLabels []string // exclude issues that have ANY of these labels
	ExcludeTypes  []string // exclude these issue types

	DescContains     string // case-insensitive substring on description
	EmptyDescription bool   // description IS NULL or empty

	Sort    string // one of: priority, created, updated, closed, id, title, type (default = priority, created)
	Reverse bool   // flip sort order

	Limit int // 0 = no limit
}

// validSortKeys enumerates the columns Sort accepts. Used both for
// ListFilter.toOrderExpr and for CLI enum validation.
var validSortKeys = []string{"priority", "created", "updated", "closed", "id", "title", "type"}

// orderExpr returns the SQL ORDER BY clause for the given Sort/Reverse.
// Empty Sort uses the default ordering (priority asc, created asc).
func (f ListFilter) orderExpr() string {
	dir := "ASC"
	if f.Reverse {
		dir = "DESC"
	}
	if f.Sort == "" {
		// Default: stable, useful ordering. Honor Reverse.
		if f.Reverse {
			return "i.priority DESC, i.created DESC"
		}
		return "i.priority ASC, i.created ASC"
	}
	for _, k := range validSortKeys {
		if k == f.Sort {
			return fmt.Sprintf("i.%s %s", f.Sort, dir)
		}
	}
	// Caller is expected to validate before calling; fall back to default.
	return "i.priority ASC, i.created ASC"
}

// applyListFilter mutates q with WHERE clauses derived from f. Shared by
// List and Count so a filter change in one stays in lock-step with the other.
func applyListFilter(q *bun.SelectQuery, f ListFilter) *bun.SelectQuery {
	if len(f.Statuses) > 0 {
		q = q.Where("i.status IN (?)", bun.In(f.Statuses))
	}
	if f.Agent != nil {
		q = q.Where("i.agent = ?", *f.Agent)
	}
	if f.Type != "" {
		q = q.Where("i.type = ?", f.Type)
	}
	if len(f.Labels) > 0 {
		q = q.Where(
			"i.id IN (SELECT issue_id FROM issue_labels WHERE label IN (?) GROUP BY issue_id HAVING COUNT(DISTINCT label) = ?)",
			bun.In(f.Labels), len(f.Labels),
		)
	}
	if len(f.LabelsAny) > 0 {
		q = q.Where(
			"i.id IN (SELECT issue_id FROM issue_labels WHERE label IN (?))",
			bun.In(f.LabelsAny),
		)
	}
	if f.NoLabels {
		q = q.Where("NOT EXISTS (SELECT 1 FROM issue_labels WHERE issue_id = i.id)")
	}
	if f.NoAssignee {
		q = q.Where("i.assignee IS NULL")
	}
	if f.TitleContains != "" {
		q = q.Where("LOWER(i.title) LIKE ?", "%"+strings.ToLower(f.TitleContains)+"%")
	}
	if f.PriorityMin != nil {
		q = q.Where("i.priority >= ?", *f.PriorityMin)
	}
	if f.PriorityMax != nil {
		q = q.Where("i.priority <= ?", *f.PriorityMax)
	}
	if f.CreatedAfter != nil {
		q = q.Where("i.created >= ?", *f.CreatedAfter)
	}
	if f.CreatedBefore != nil {
		q = q.Where("i.created <= ?", *f.CreatedBefore)
	}
	if f.UpdatedAfter != nil {
		q = q.Where("i.updated >= ?", *f.UpdatedAfter)
	}
	if f.UpdatedBefore != nil {
		q = q.Where("i.updated <= ?", *f.UpdatedBefore)
	}
	if len(f.IDs) > 0 {
		q = q.Where("i.id IN (?)", bun.In(f.IDs))
	}
	if f.Deferred {
		q = q.Where("i.defer_until IS NOT NULL AND i.defer_until > ?", now())
	}
	if f.Overdue {
		q = q.Where("i.defer_until IS NOT NULL AND i.defer_until <= ? AND i.status != 'closed'", now())
	}
	if f.LabelPattern != "" {
		q = q.Where(
			"i.id IN (SELECT issue_id FROM issue_labels WHERE label GLOB ?)",
			f.LabelPattern,
		)
	}
	if len(f.ExcludeLabels) > 0 {
		q = q.Where(
			"i.id NOT IN (SELECT issue_id FROM issue_labels WHERE label IN (?))",
			bun.In(f.ExcludeLabels),
		)
	}
	if len(f.ExcludeTypes) > 0 {
		q = q.Where("i.type NOT IN (?)", bun.In(f.ExcludeTypes))
	}
	if f.DescContains != "" {
		q = q.Where("LOWER(i.description) LIKE ?", "%"+strings.ToLower(f.DescContains)+"%")
	}
	if f.EmptyDescription {
		q = q.Where("i.description IS NULL OR i.description = ''")
	}
	return q
}

func (s *Store) List(ctx context.Context, f ListFilter) ([]Issue, error) {
	var issues []Issue
	q := s.db.NewSelect().Model(&issues).OrderExpr(f.orderExpr())
	q = applyListFilter(q, f)
	if f.Limit > 0 {
		q = q.Limit(f.Limit)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return issues, nil
}

// Count returns the number of issues matching the filter. Honors every
// dimension of ListFilter except Limit.
func (s *Store) Count(ctx context.Context, f ListFilter) (int, error) {
	q := s.db.NewSelect().Model((*Issue)(nil))
	q = applyListFilter(q, f)
	return q.Count(ctx)
}

// Stats is a snapshot of issue counts grouped by various dimensions.
type Stats struct {
	Status map[string]int `json:"status"`
	Agents map[string]int `json:"agents"` // nil agent rendered as "<none>"
	Types  map[string]int `json:"types"`
}

// Stats returns counts grouped by status, agent (NULL → "<none>"), and type.
func (s *Store) Stats(ctx context.Context) (Stats, error) {
	out := Stats{Status: map[string]int{}, Agents: map[string]int{}, Types: map[string]int{}}

	type row struct {
		Key   *string `bun:"key"`
		Count int     `bun:"count"`
	}

	scan := func(col string, dest map[string]int) error {
		var rows []row
		err := s.db.NewSelect().
			Model((*Issue)(nil)).
			ColumnExpr(col+" AS key").
			ColumnExpr("COUNT(*) AS count").
			GroupExpr(col).
			Scan(ctx, &rows)
		if err != nil {
			return err
		}
		for _, r := range rows {
			k := "<none>"
			if r.Key != nil {
				k = *r.Key
			}
			dest[k] = r.Count
		}
		return nil
	}
	if err := scan("i.status", out.Status); err != nil {
		return Stats{}, err
	}
	if err := scan("i.agent", out.Agents); err != nil {
		return Stats{}, err
	}
	if err := scan("i.type", out.Types); err != nil {
		return Stats{}, err
	}
	return out, nil
}

// Blocked returns open issues that have at least one non-closed dependency.
// Inverse of Ready. Same agent-lane semantics.
func (s *Store) Blocked(ctx context.Context, limit int, agent *string) ([]Issue, error) {
	if limit <= 0 {
		limit = 50
	}
	var issues []Issue
	q := s.db.NewSelect().Model(&issues).
		Where("i.status = 'open'").
		Where("EXISTS (SELECT 1 FROM deps d JOIN issues p ON p.id = d.parent_id WHERE d.child_id = i.id AND p.status != 'closed')").
		OrderExpr("i.priority ASC, i.created ASC").
		Limit(limit)
	if agent == nil {
		q = q.Where("i.agent IS NULL")
	} else {
		q = q.Where("i.agent = ?", *agent)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return issues, nil
}

// Reopen transitions a closed issue back to open and clears the closed
// timestamp. Symmetric with MarkClosed.
func (s *Store) Reopen(ctx context.Context, id string) (Issue, error) {
	res, err := s.db.NewUpdate().
		Model((*Issue)(nil)).
		Set("status = 'open'").
		Set("closed = NULL").
		Set("updated = ?", now()).
		Where("id = ? AND status = 'closed'", id).
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

// ---- Labels ----

// AddLabels attaches one or more labels to an issue. No-op for empty list.
// Returns ErrNotFound if the issue does not exist. Rejects empty-string
// labels — a label with no characters is almost certainly a quoting
// mistake and persists as a blank row in `label ls`.
func (s *Store) AddLabels(ctx context.Context, issueID string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	for _, l := range labels {
		if l == "" {
			return errors.New("label cannot be empty")
		}
	}
	if err := s.exists(ctx, issueID); err != nil {
		return err
	}
	rows := make([]IssueLabel, len(labels))
	for i, l := range labels {
		rows[i] = IssueLabel{IssueID: issueID, Label: l}
	}
	_, err := s.db.NewInsert().Model(&rows).On("CONFLICT DO NOTHING").Exec(ctx)
	return err
}

// RemoveLabels detaches labels from an issue. No-op for empty list or for
// labels that are not present. Returns ErrNotFound if the issue itself
// doesn't exist (rather than silently no-op'ing on a typo'd ID).
func (s *Store) RemoveLabels(ctx context.Context, issueID string, labels []string) error {
	if err := s.exists(ctx, issueID); err != nil {
		return err
	}
	if len(labels) == 0 {
		return nil
	}
	_, err := s.db.NewDelete().
		Model((*IssueLabel)(nil)).
		Where("issue_id = ?", issueID).
		Where("label IN (?)", bun.In(labels)).
		Exec(ctx)
	return err
}

// LabelsForIssue returns the labels on a single issue, alphabetically.
// Returns ErrNotFound if the issue doesn't exist (distinct from "issue
// exists but has no labels", which returns an empty slice).
func (s *Store) LabelsForIssue(ctx context.Context, issueID string) ([]string, error) {
	if err := s.exists(ctx, issueID); err != nil {
		return nil, err
	}
	var labels []string
	err := s.db.NewSelect().
		Model((*IssueLabel)(nil)).
		Column("label").
		Where("issue_id = ?", issueID).
		OrderExpr("label").
		Scan(ctx, &labels)
	return labels, err
}

// LoadLabels returns a map id -> []labels for the given issue IDs in one query.
// Used for batch display in list/show output.
func (s *Store) LoadLabels(ctx context.Context, ids []string) (map[string][]string, error) {
	out := make(map[string][]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []IssueLabel
	err := s.db.NewSelect().
		Model(&rows).
		Where("issue_id IN (?)", bun.In(ids)).
		OrderExpr("issue_id, label").
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.IssueID] = append(out[r.IssueID], r.Label)
	}
	return out, nil
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

// UpsertDep inserts a dep edge, ignoring conflicts. Skips AddDep's
// existence and cycle checks: callers (import) trust the input.
func (s *Store) UpsertDep(ctx context.Context, child, parent string) error {
	_, err := s.db.NewInsert().
		Model(&Dep{ChildID: child, ParentID: parent}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

// AddComment appends a new comment to an issue. Validates the issue
// exists. Returns the inserted Comment with its allocated ID and
// `created` timestamp.
func (s *Store) AddComment(ctx context.Context, issueID, author, body string) (Comment, error) {
	if author == "" {
		return Comment{}, errors.New("author required")
	}
	if body == "" {
		return Comment{}, errors.New("body required")
	}
	if err := s.exists(ctx, issueID); err != nil {
		return Comment{}, err
	}
	c := Comment{IssueID: issueID, Author: author, Body: body, Created: now()}
	if _, err := s.db.NewInsert().Model(&c).Exec(ctx); err != nil {
		return Comment{}, err
	}
	return c, nil
}

// Comments returns all comments on an issue in chronological order.
// Returns ErrNotFound if the issue itself doesn't exist (distinct from
// "no comments yet", which returns an empty slice).
func (s *Store) Comments(ctx context.Context, issueID string) ([]Comment, error) {
	if err := s.exists(ctx, issueID); err != nil {
		return nil, err
	}
	var cs []Comment
	err := s.db.NewSelect().
		Model(&cs).
		Where("issue_id = ?", issueID).
		OrderExpr("id ASC").
		Scan(ctx)
	return cs, err
}

// RemoveComment deletes a comment by its numeric ID. Returns ErrNotFound
// if no such comment exists.
func (s *Store) RemoveComment(ctx context.Context, commentID int64) error {
	res, err := s.db.NewDelete().
		Model((*Comment)(nil)).
		Where("id = ?", commentID).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpsertComment inserts a comment with an explicit ID (used by import).
// On conflict, updates every field.
func (s *Store) UpsertComment(ctx context.Context, c Comment) error {
	_, err := s.db.NewInsert().Model(&c).
		On("CONFLICT (id) DO UPDATE").
		Set("issue_id = EXCLUDED.issue_id").
		Set("author = EXCLUDED.author").
		Set("body = EXCLUDED.body").
		Set("created = EXCLUDED.created").
		Exec(ctx)
	return err
}

// AllComments returns every comment, ordered deterministically for export.
func (s *Store) AllComments(ctx context.Context) ([]Comment, error) {
	var cs []Comment
	err := s.db.NewSelect().Model(&cs).OrderExpr("id ASC").Scan(ctx)
	return cs, err
}

// ---- KV ----

// KVSet upserts a key-value pair. Replaces the value if the key exists.
func (s *Store) KVSet(ctx context.Context, key, value string) error {
	if key == "" {
		return errors.New("key required")
	}
	kv := KV{Key: key, Value: value}
	_, err := s.db.NewInsert().Model(&kv).
		On("CONFLICT (key) DO UPDATE").
		Set("value = EXCLUDED.value").
		Exec(ctx)
	return err
}

// KVGet returns the value for a key, or ErrKVNotFound if missing.
func (s *Store) KVGet(ctx context.Context, key string) (string, error) {
	var kv KV
	err := s.db.NewSelect().Model(&kv).Where("key = ?", key).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrKVNotFound
	}
	if err != nil {
		return "", err
	}
	return kv.Value, nil
}

// KVDelete removes a key. Returns ErrKVNotFound if it wasn't present.
func (s *Store) KVDelete(ctx context.Context, key string) error {
	res, err := s.db.NewDelete().Model((*KV)(nil)).Where("key = ?", key).Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrKVNotFound
	}
	return nil
}

// KVList returns every entry, alphabetised by key. Also used for export.
func (s *Store) KVList(ctx context.Context) ([]KV, error) {
	var kvs []KV
	err := s.db.NewSelect().Model(&kvs).OrderExpr("key ASC").Scan(ctx)
	return kvs, err
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

// DBVersion returns the PRAGMA user_version of the underlying database.
func (s *Store) DBVersion(ctx context.Context) (int, error) {
	var v int
	err := s.db.NewRaw("PRAGMA user_version").Scan(ctx, &v)
	return v, err
}

// DoctorReport is the result of a health check.
type DoctorReport struct {
	DBSchemaVersion   int      `json:"db_schema_version"`
	CodeSchemaVersion int      `json:"code_schema_version"`
	ForeignKeyOK      bool     `json:"foreign_key_ok"`
	ForeignKeyErrors  []string `json:"foreign_key_errors,omitempty"`
	OrphanedLabels    int      `json:"orphaned_labels"`
	OrphanedDeps      int      `json:"orphaned_deps"`
	StuckInProgress   int      `json:"stuck_in_progress"`
	StuckThresholdH   int      `json:"stuck_threshold_hours"`
	ClosedButDeferred int      `json:"closed_but_deferred"`
	InvalidStatus     int      `json:"invalid_status"`
	InvalidType       int      `json:"invalid_type"`
	InvalidPriority   int      `json:"invalid_priority"`
}

// OK returns true when nothing is wrong.
func (d DoctorReport) OK() bool {
	return d.DBSchemaVersion == d.CodeSchemaVersion &&
		d.ForeignKeyOK &&
		d.OrphanedLabels == 0 &&
		d.OrphanedDeps == 0 &&
		d.StuckInProgress == 0 &&
		d.ClosedButDeferred == 0 &&
		d.InvalidStatus == 0 &&
		d.InvalidType == 0 &&
		d.InvalidPriority == 0
}

// Doctor runs integrity checks against the live database. Issues
// "stuck" longer than stuckThresholdHours are flagged. Pass 0 to
// disable the stuck check.
func (s *Store) Doctor(ctx context.Context, stuckThresholdHours int) (DoctorReport, error) {
	r := DoctorReport{
		CodeSchemaVersion: SchemaVersion(),
		StuckThresholdH:   stuckThresholdHours,
	}
	v, err := s.DBVersion(ctx)
	if err != nil {
		return r, err
	}
	r.DBSchemaVersion = v

	// PRAGMA foreign_key_check returns one row per violation; empty = OK.
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var table, rowid, parent, fkid sql.NullString
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err == nil {
			r.ForeignKeyErrors = append(r.ForeignKeyErrors,
				fmt.Sprintf("%s rowid=%s ref=%s fk=%s", table.String, rowid.String, parent.String, fkid.String))
		}
	}
	r.ForeignKeyOK = len(r.ForeignKeyErrors) == 0

	// FKs with CASCADE should prevent orphans, but check anyway as a belt-and-braces.
	if err := s.db.NewRaw("SELECT COUNT(*) FROM issue_labels WHERE issue_id NOT IN (SELECT id FROM issues)").Scan(ctx, &r.OrphanedLabels); err != nil {
		return r, err
	}
	if err := s.db.NewRaw("SELECT COUNT(*) FROM deps WHERE child_id NOT IN (SELECT id FROM issues) OR parent_id NOT IN (SELECT id FROM issues)").Scan(ctx, &r.OrphanedDeps); err != nil {
		return r, err
	}

	if stuckThresholdHours > 0 {
		cutoff := now() - int64(stuckThresholdHours)*3600
		if err := s.db.NewRaw("SELECT COUNT(*) FROM issues WHERE status = 'in_progress' AND updated < ?", cutoff).Scan(ctx, &r.StuckInProgress); err != nil {
			return r, err
		}
	}

	if err := s.db.NewRaw("SELECT COUNT(*) FROM issues WHERE status = 'closed' AND defer_until IS NOT NULL").Scan(ctx, &r.ClosedButDeferred); err != nil {
		return r, err
	}

	// Pre-existing rows that escape current validation. The store
	// itself rejects bad values at write time, but older rows (or
	// imports that bypass Create/Update) may still be in violation.
	if r.InvalidStatus, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.status NOT IN (?)", bun.In(ValidStatuses)).Count(ctx); err != nil {
		return r, err
	}
	if r.InvalidType, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.type NOT IN (?)", bun.In(ValidTypes)).Count(ctx); err != nil {
		return r, err
	}
	if r.InvalidPriority, err = s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.priority < ? OR i.priority > ?", MinPriority, MaxPriority).Count(ctx); err != nil {
		return r, err
	}
	return r, nil
}

// AllDeps returns every dep edge, ordered for deterministic export.
func (s *Store) AllDeps(ctx context.Context) ([]Dep, error) {
	var deps []Dep
	err := s.db.NewSelect().
		Model(&deps).
		OrderExpr("child_id, parent_id").
		Scan(ctx)
	return deps, err
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

// readyQuery applies the shared WHERE/ORDER for ready-issue selection.
// Excludes issues whose defer_until is still in the future.
func (s *Store) readyQuery(agent *string) *bun.SelectQuery {
	q := s.db.NewSelect().Model((*Issue)(nil)).
		Where("i.status = 'open' AND i.assignee IS NULL").
		Where("(i.defer_until IS NULL OR i.defer_until <= ?)", now()).
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
