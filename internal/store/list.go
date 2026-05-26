package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

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
	return "i.priority ASC, i.created ASC"
}

// applyListFilter mutates q with WHERE clauses derived from f. Shared by
// List and Count so a filter change in one stays in lock-step with the other.
func applyListFilter(q *bun.SelectQuery, f ListFilter) *bun.SelectQuery {
	if len(f.Statuses) > 0 {
		q = q.Where("i.status IN (?)", bun.In(f.Statuses))
	}
	if f.Agent != nil {
		// `-a X` means "X's work" from a user POV. clu tracks two
		// columns (agent = lane / routing; assignee = currently
		// working on it), and matching only `agent` hides issues
		// that were assigned to X without changing the lane (e.g.
		// `clu assign <id> X`). Until the columns are unified, the
		// filter unions them so the user-facing model is one name.
		q = q.Where("i.agent = ? OR i.assignee = ?", *f.Agent, *f.Agent)
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

// MaxUpdated returns the largest `updated` timestamp across all
// issues, or 0 if there are no rows. Used by the HTTP server's
// change-stream poll loop to detect cross-process writes (e.g. a
// `clu close <id>` run in another terminal while a web client is
// connected). Cheap — single aggregate, no scan.
func (s *Store) MaxUpdated(ctx context.Context) (int64, error) {
	var v int64
	err := s.db.NewSelect().
		Model((*Issue)(nil)).
		ColumnExpr("COALESCE(MAX(updated), 0)").
		Scan(ctx, &v)
	if err != nil {
		return 0, err
	}
	return v, nil
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
		q = q.Where("i.agent IS NULL AND i.assignee IS NULL")
	} else {
		// Mirror the unified `-a` filter from List: match either column.
		q = q.Where("i.agent = ? OR i.assignee = ?", *agent, *agent)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, err
	}
	return issues, nil
}
