package creation

import (
	"context"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// Invocation carries attribution resolved for one project creation request.
type Invocation struct {
	// actor is normalized attribution supplied at the process boundary.
	actor string
}

// NewInvocation normalizes project creation attribution.
func NewInvocation(actor string) Invocation {
	return Invocation{actor: strings.TrimSpace(actor)}
}

// Actor returns the normalized invocation actor.
func (i Invocation) Actor() string { return i.actor }

// Request supplies caller-selected project creation values.
type Request struct {
	// Name is the new project's user-visible name.
	Name string

	// Prefix explicitly selects the project issue ID prefix when present.
	Prefix *string
}

// Creation contains the project values selected for atomic persistence.
type Creation struct {
	// Name is the new project's user-visible name.
	Name string

	// Prefix is the project-level issue ID prefix to persist.
	// Nil preserves inheritance from the physical store.
	Prefix *configuration.Prefix
}

//go:generate go tool mockgen -destination mocks_test.go -package creation -typed -write_package_comment=false . Configuration,Projects

// Configuration supplies validated physical-store configuration.
type Configuration interface {
	// ReadStoreConfiguration returns the active store-level overrides.
	ReadStoreConfiguration(context.Context) (configuration.Overrides, error)
}

// Projects persists finite project creation operations.
type Projects interface {
	// CreateProject atomically establishes one project.
	CreateProject(context.Context, Creation) (*project.State, error)
}

// Service owns project creation policy and persistence coordination.
type Service struct {
	// configuration supplies the active physical-store prefix.
	configuration Configuration

	// projects owns the atomic project persistence boundary.
	projects Projects
}

// NewService constructs project creation operations.
func NewService(configuration Configuration, projects Projects) *Service {
	must.NotBeNilf(configuration, "project creation Configuration is required")
	must.NotBeNilf(projects, "project creation Projects is required")
	return &Service{configuration: configuration, projects: projects}
}

// CreateProject establishes one project without creating a board.
func (s *Service) CreateProject(
	ctx context.Context,
	_ Invocation,
	request Request,
) (*project.State, error) {
	store, err := s.configuration.ReadStoreConfiguration(ctx)
	if err != nil {
		return nil, fmt.Errorf("read store configuration: %w", err)
	}
	prefix, err := configuration.SelectProjectCreationPrefix(
		request.Name,
		request.Prefix,
		store,
	)
	if err != nil {
		return nil, errkind.Wrap(errkind.InvalidInput, err)
	}
	return s.projects.CreateProject(ctx, Creation{
		Name:   request.Name,
		Prefix: prefix,
	})
}
