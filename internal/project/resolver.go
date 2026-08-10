package project

import (
	"context"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
)

// EditNameRequest identifies one project by stable ID and supplies its
// replacement name. The edit preserves every other State field.
type EditNameRequest struct {
	// ProjectID identifies the persisted project to edit.
	ProjectID ID

	// Name replaces the user-visible project name. Surrounding whitespace is
	// trimmed; a blank value is invalid.
	Name string
}

//go:generate go tool mockgen -destination mocks_test.go -package project -typed -write_package_comment=false . Projects,Changes

// Projects provides the project states required by Service.
type Projects interface {
	// ListProjects returns every project from one coherent catalog read in
	// deterministic order.
	ListProjects(context.Context) ([]*State, error)
}

// Changes owns committed, ID-addressed project mutations.
// Changes does not interpret project names as selectors.
// Successful methods return the authoritative State visible after commit.
type Changes interface {
	// EditProjectName loads and validates the identified project within the
	// same change that persists its replacement name. A missing project returns
	// NotFound, and an invalid name returns InvalidInput; either error leaves
	// stored state and the canonical revision unchanged. A changed name and its
	// canonical revision become visible together, while a normalized no-op
	// returns current State without publishing a revision.
	EditProjectName(context.Context, EditNameRequest) (*State, error)
}

// Service exposes finite project catalog reads, selector resolution, and
// ID-addressed name edits in one store. Callers that start with a project name
// resolve it before constructing an edit request; stable IDs may be used
// directly.
type Service struct {
	// projects supplies one coherent project catalog read per resolution.
	projects Projects

	// changes commits mutations after the caller has resolved a stable ID.
	changes Changes
}

// NewService constructs project catalog and mutation operations.
// It panics when either dependency is nil because process composition must
// provide both collaborators.
func NewService(projects Projects, changes Changes) *Service {
	must.NotBeNilf(projects, "project Projects is required")
	must.NotBeNilf(changes, "project Changes is required")
	return &Service{projects: projects, changes: changes}
}

// EditName delegates an ID-addressed rename and returns its committed State.
// A missing project returns NotFound, and an invalid name returns InvalidInput.
// EditName does not interpret ProjectID as a name selector; callers that have a
// name must resolve a Selector first.
func (s *Service) EditName(
	ctx context.Context,
	request EditNameRequest,
) (*State, error) {
	return s.changes.EditProjectName(ctx, request)
}

// List returns every project in deterministic catalog order.
func (s *Service) List(ctx context.Context) ([]*State, error) {
	return s.projects.ListProjects(ctx)
}

// Resolve returns the project selected by stable ID or exact name.
// A nil selector requires the store to contain exactly one project. No match
// returns NotFound; multiple possible projects return Conflict.
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
