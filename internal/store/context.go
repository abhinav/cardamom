package store

import "context"

// AncestorContext returns the transitive dependency (ancestor) IDs of id —
// everything id depends on, directly or transitively — ordered most-upstream
// first (roots before direct parents). Read top-to-bottom it's the story
// leading up to id: prerequisites, then what built on them.
//
// maxDepth > 0 caps how far up the chain to walk (1 = direct parents only);
// 0 means unlimited. The graph is acyclic, so the walk always terminates.
func (s *Store) AncestorContext(ctx context.Context, id string, maxDepth int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
        WITH RECURSIVE anc(id, depth) AS (
            SELECT parent_id, 1 FROM deps WHERE child_id = ?
            UNION
            SELECT d.parent_id, anc.depth + 1
            FROM deps d JOIN anc ON d.child_id = anc.id
        )
        SELECT id, MAX(depth) AS d FROM anc GROUP BY id ORDER BY d DESC, id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var aid string
		var depth int
		if err := rows.Scan(&aid, &depth); err != nil {
			return nil, err
		}
		if maxDepth > 0 && depth > maxDepth {
			continue
		}
		ids = append(ids, aid)
	}
	return ids, rows.Err()
}
