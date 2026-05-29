package store

import "context"

// closeSatisfiedMilestones is clu's one reactive behavior: after an issue
// closes, any milestone that depends on it whose dependencies are now ALL
// closed closes too — recursively, since closing a milestone can satisfy a
// higher one. This is what makes `clu run` / `clu batch --group` umbrellas
// self-complete and phase-boundary milestones advance automatically.
//
// Rules:
//   - only type="milestone" issues auto-close;
//   - a milestone closes only when every dependency is closed — a CANCELLED
//     dependency does NOT satisfy it (the set didn't all succeed), so it
//     stays open for a human/agent to resolve;
//   - reopening is not cascaded (a milestone that already auto-closed stays
//     closed even if a dependency is later reopened — rare, and un-cascading
//     is deliberately out of scope).
//
// Best-effort, like recordEvent: a failure here never fails the originating
// close. Cascade terminates because the dependency graph is acyclic.
func (s *Store) closeSatisfiedMilestones(ctx context.Context, closedID string) {
	// Dependents: issues C with an edge (child=C, parent=closedID).
	var childIDs []string
	if err := s.db.NewSelect().Model((*Dep)(nil)).
		Column("child_id").
		Where("parent_id = ?", closedID).
		Scan(ctx, &childIDs); err != nil {
		return
	}
	for _, cid := range childIDs {
		c, err := s.Get(ctx, cid)
		if err != nil {
			continue
		}
		if c.Type != "milestone" || c.Status == "closed" || c.Status == "cancelled" {
			continue
		}
		done, err := s.allParentsClosed(ctx, cid)
		if err != nil || !done {
			continue
		}
		t := now()
		res, err := s.db.NewUpdate().
			Model((*Issue)(nil)).
			Set("status = 'closed'").
			Set("closed = ?", t).
			Set("updated = ?", t).
			Set("defer_until = NULL").
			Where("id = ? AND status NOT IN ('closed', 'cancelled')", cid).
			Exec(ctx)
		if err != nil {
			continue
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue // someone else closed/cancelled it concurrently
		}
		// auto:true distinguishes a milestone self-completion from a manual
		// close in `clu log`.
		s.recordEvent(ctx, cid, "closed", map[string]any{"status": "closed", "auto": true})
		// Recurse: this milestone closing may satisfy a higher milestone.
		s.closeSatisfiedMilestones(ctx, cid)
	}
}

// allParentsClosed reports whether every dependency of id is closed. A
// cancelled (or still-open) parent counts as not-closed, so a milestone with
// any cancelled dependency will not auto-close.
func (s *Store) allParentsClosed(ctx context.Context, id string) (bool, error) {
	n, err := s.db.NewSelect().
		Model((*Dep)(nil)).
		Join("JOIN issues p ON p.id = dep.parent_id").
		Where("dep.child_id = ?", id).
		Where("p.status != 'closed'").
		Count(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}
