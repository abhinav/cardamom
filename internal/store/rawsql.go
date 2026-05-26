package store

import "context"

// RawQuery runs an arbitrary read query and returns the column names
// plus the rows as raw []any (one slice per row). Callers handle NULL
// by checking for a nil element. Used by `clu sql` so we don't have to
// pre-declare a model — the whole point is ad-hoc.
//
// Schema is internal: queries that work today can break across migration
// versions. Don't bake them into shared tooling.
func (s *Store) RawQuery(ctx context.Context, query string, args ...any) ([]string, [][]any, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	var out [][]any
	for rows.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		out = append(out, holders)
	}
	return cols, out, rows.Err()
}

// RawExec runs an arbitrary write statement (UPDATE/DELETE/INSERT/DDL)
// and returns the rows-affected count. Use for `clu sql --write`.
func (s *Store) RawExec(ctx context.Context, stmt string, args ...any) (int64, error) {
	res, err := s.db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
