package project

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/project"
	projectcreation "go.abhg.dev/cardamom/internal/project/creation"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CreateProject atomically establishes one project at a canonical revision.
func (r *Repository) CreateProject(
	ctx context.Context,
	request projectcreation.Creation,
) (*project.State, error) {
	projectID, err := r.idSource.NewID("project")
	if err != nil {
		return nil, fmt.Errorf("generate project identity: %w", err)
	}
	state, err := project.Load(project.Snapshot{
		ID:      project.ID(projectID),
		Name:    request.Name,
		Created: r.clock(),
	})
	if err != nil {
		return nil, err
	}
	var prefix *string
	if request.Prefix != nil {
		prefix = new(request.Prefix.String())
	}
	err = r.commitRevision(ctx, func(change *store.Change) error {
		return query.New(change).ProjectCreateProject(
			ctx,
			query.ProjectCreateProjectParams{
				ID:            state.ID().String(),
				Name:          state.Name(),
				CreatedAt:     state.Created(),
				IssueIDPrefix: prefix,
			},
		)
	})
	if err != nil {
		return nil, err
	}
	return state, nil
}
