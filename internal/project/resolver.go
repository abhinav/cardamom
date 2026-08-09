package project

import (
	"context"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

//go:generate go tool mockgen -destination mocks_test.go -package project -typed -write_package_comment=false . Projects

// Projects provides the project states required by Resolver.
type Projects interface {
	// ListProjects returns every project in deterministic order.
	ListProjects(context.Context) ([]*State, error)
}

// Service lists and selects projects from the store catalog.
type Service struct {
	// projects supplies one coherent project catalog read per resolution.
	projects Projects
}

// NewService constructs project catalog operations.
func NewService(projects Projects) *Service {
	must.NotBeNilf(projects, "project Projects is required")
	return &Service{projects: projects}
}

// List returns every project in deterministic catalog order.
func (s *Service) List(ctx context.Context) ([]*State, error) {
	return s.projects.ListProjects(ctx)
}

// Resolve selects an explicit project or the store's sole project.
func (s *Service) Resolve(ctx context.Context, selector *Selector) (*State, error) {
	projects, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	if selector == nil {
		switch len(projects) {
		case 0:
			return nil, errkind.Errorf(errkind.NotFound, "project not found")
		case 1:
			return projects[0], nil
		default:
			return nil, errkind.Errorf(errkind.Conflict, "project selection is ambiguous")
		}
	}
	var matches []*State
	for _, candidate := range projects {
		if selector.Matches(candidate) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return nil, errkind.Errorf(errkind.NotFound, "project %q not found", selector.String())
	case 1:
		return matches[0], nil
	default:
		return nil, errkind.Errorf(
			errkind.Conflict,
			"project %q is ambiguous; use an ID",
			selector.String(),
		)
	}
}
