package store

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

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

// UpsertDep inserts a dep edge, ignoring conflicts. Skips AddDep's
// existence and cycle checks: callers (import) trust the input.
func (s *Store) UpsertDep(ctx context.Context, child, parent string) error {
	_, err := s.db.NewInsert().
		Model(&Dep{ChildID: child, ParentID: parent}).
		On("CONFLICT DO NOTHING").
		Exec(ctx)
	return err
}

// RemoveDep deletes a child->parent dependency edge. Returns
// ErrDepNotFound if no such edge exists; lets the CLI distinguish
// "actually removed something" from "no-op" in scripted contexts.
func (s *Store) RemoveDep(ctx context.Context, child, parent string) error {
	res, err := s.db.NewDelete().
		Model((*Dep)(nil)).
		Where("child_id = ? AND parent_id = ?", child, parent).
		Exec(ctx)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDepNotFound
	}
	return nil
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

// IDsBlocked returns the subset of `ids` that have at least one non-closed
// parent — i.e. the issues that are effectively "blocked" regardless of
// their own status. Empty input returns an empty (non-nil) map.
//
// Used by `cli list` / `cli ready` to surface blocked-ness as a derived
// status without storing it on the issue row.
func (s *Store) IDsBlocked(ctx context.Context, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ChildID string `bun:"child_id"`
	}
	err := s.db.NewSelect().
		Model((*Dep)(nil)).
		ColumnExpr("DISTINCT child_id").
		Join("JOIN issues p ON p.id = dep.parent_id").
		Where("dep.child_id IN (?)", bun.In(ids)).
		Where("p.status != 'closed'").
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.ChildID] = true
	}
	return out, nil
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
