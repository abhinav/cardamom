package project

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListProjects returns all projects in stable name and identity order.
func (r *Repository) ListProjects(ctx context.Context) (out []*project.State, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	rows, err := query.New(view).ProjectListProjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		namespace, err := project.Load(project.Snapshot{
			ID:      project.ID(row.ID),
			Name:    row.Name,
			Created: row.CreatedAt,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, namespace)
	}
	return out, nil
}
