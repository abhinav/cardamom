package inspection

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// Detail is the complete project-scoped read model returned by Show.
type Detail struct {
	// Project is the selected project's immutable metadata.
	Project *project.State

	// Configuration is effective through the selected project layer.
	Configuration configuration.ProjectView

	// Boards contains only boards owned by Project, in deterministic order.
	Boards []*board.State
}

// Projects resolves stable project IDs and exact names.
type Projects interface {
	// Resolve selects one project, preserving duplicate-name ambiguity.
	Resolve(context.Context, *project.Selector) (*project.State, error)
}

// Configuration resolves effective configuration without a board selection.
type Configuration interface {
	// ResolveProject applies built-in, store, and project precedence.
	// A missing project returns NotFound.
	ResolveProject(context.Context, project.ID) (configuration.ProjectView, error)
}

// Boards supplies project-scoped board inventory.
type Boards interface {
	// ListProjectBoards returns one project's boards in deterministic order.
	ListProjectBoards(context.Context, project.ID) ([]*board.State, error)
}

// Service composes the project metadata, configuration, and board inventory
// needed by one project inspection.
type Service struct {
	projects      Projects
	configuration Configuration
	boards        Boards
}

// NewService constructs project inspection operations. It panics when a
// dependency is nil because process composition must provide all readers.
func NewService(
	projects Projects,
	configuration Configuration,
	boards Boards,
) *Service {
	must.NotBeNilf(projects, "project inspection Projects is required")
	must.NotBeNilf(configuration, "project inspection Configuration is required")
	must.NotBeNilf(boards, "project inspection Boards is required")
	return &Service{
		projects: projects, configuration: configuration, boards: boards,
	}
}

// Show resolves one project and returns its project-scoped read model. Selector
// may use stable identity or exact name; an ambiguous name fails before
// configuration or board reads begin.
func (s *Service) Show(
	ctx context.Context,
	selector project.Selector,
) (Detail, error) {
	selected, err := s.projects.Resolve(ctx, &selector)
	if err != nil {
		return Detail{}, err
	}
	resolved, err := s.configuration.ResolveProject(ctx, selected.ID())
	if err != nil {
		return Detail{}, fmt.Errorf("resolve project configuration: %w", err)
	}
	boards, err := s.boards.ListProjectBoards(ctx, selected.ID())
	if err != nil {
		return Detail{}, fmt.Errorf("list project boards: %w", err)
	}
	return Detail{
		Project: selected, Configuration: resolved, Boards: boards,
	}, nil
}
