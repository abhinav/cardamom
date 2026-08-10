package configuration

import (
	"context"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// Scope identifies one layer in the configuration hierarchy.
type Scope uint8

const (
	_ Scope = iota

	// ScopeBuiltIn identifies compiled defaults.
	ScopeBuiltIn

	// ScopeStore identifies local physical-store YAML.
	ScopeStore

	// ScopeProject identifies synchronized project overrides.
	ScopeProject

	// ScopeBoard identifies synchronized board overrides.
	ScopeBoard
)

// String returns the stable scope name.
func (s Scope) String() string {
	switch s {
	case ScopeBuiltIn:
		return "built_in"
	case ScopeStore:
		return "store"
	case ScopeProject:
		return "project"
	case ScopeBoard:
		return "board"
	default:
		return ""
	}
}

// Source identifies the layer that supplies one effective field.
type Source struct {
	// Scope identifies the source layer.
	Scope Scope

	// Identity identifies the concrete store, project, or board.
	Identity string
}

// Origins mirrors Configuration with one source for every effective field.
type Origins struct {
	// Issue contains origins for issue policy.
	Issue IssueOrigins

	// Attachment contains origins for attachment policy.
	Attachment AttachmentOrigins
}

// IssueOrigins contains origins for issue policy.
type IssueOrigins struct {
	// ID contains origins for issue identity policy.
	ID IssueIDOrigins

	// Summary contains origins for summary policy.
	Summary SummaryOrigins
}

// IssueIDOrigins contains origins for issue identity policy.
type IssueIDOrigins struct {
	// Prefix identifies the effective prefix source.
	Prefix Source

	// Strategy identifies the effective strategy source.
	Strategy Source
}

// SummaryOrigins contains origins for summary policy.
type SummaryOrigins struct {
	// MaxBytes identifies the effective summary-limit source.
	MaxBytes Source
}

// AttachmentOrigins contains origins for attachment policy.
type AttachmentOrigins struct {
	// MaxBytes identifies the effective attachment-limit source.
	MaxBytes Source
}

// Layer is one optional configuration layer and its concrete identity.
type Layer struct {
	// Source identifies the layer scope and owner.
	Source Source

	// Overrides contains values explicitly set at this layer.
	Overrides Overrides
}

// View contains the ordered layers and fully resolved configuration.
type View struct {
	// BuiltIn contains Cardamom's compiled defaults.
	BuiltIn Configuration

	// Store contains local physical-store overrides.
	Store Layer

	// Project contains logical project overrides.
	Project Layer

	// Board contains logical board overrides.
	Board Layer

	// Effective contains the highest-precedence value for every field.
	Effective Configuration

	// Origins identifies the source of every effective field.
	Origins Origins
}

// ProjectView represents effective configuration at project scope.
// It deliberately omits a board layer; Effective and Origins resolve only
// built-in, store, and project precedence.
type ProjectView struct {
	// BuiltIn contains Cardamom's compiled defaults.
	BuiltIn Configuration

	// Store contains local physical-store overrides.
	Store Layer

	// Project contains logical project overrides.
	Project Layer

	// Effective contains the highest-precedence value for every field.
	Effective Configuration

	// Origins identifies the source of every effective field.
	Origins Origins
}

// DatabaseLayers contains the project and board layers for one selected board.
type DatabaseLayers struct {
	// ProjectID identifies the selected board's project.
	ProjectID project.ID

	// Project contains project-level overrides.
	Project Overrides

	// Board contains board-level overrides.
	Board Overrides
}

//go:generate go tool mockgen -destination mocks_test.go -package configuration -typed -write_package_comment=false . Store,Repository

// Store persists the optional local physical-store layer.
type Store interface {
	// ReadStoreConfiguration rereads the current store overrides.
	ReadStoreConfiguration(context.Context) (Overrides, error)

	// UpdateStoreConfiguration atomically applies one finite patch.
	UpdateStoreConfiguration(context.Context, Patch) (Overrides, error)
}

// Repository persists logical project and board configuration.
type Repository interface {
	// ReadConfigurationLayers returns coherent project and board layers.
	ReadConfigurationLayers(context.Context, board.ID) (DatabaseLayers, error)

	// ReadProjectConfiguration returns one project's override layer without
	// requiring the project to contain a board.
	ReadProjectConfiguration(context.Context, project.ID) (Overrides, error)

	// UpdateProjectConfiguration atomically applies one project patch.
	UpdateProjectConfiguration(
		context.Context,
		project.ID,
		Patch,
	) (Overrides, error)

	// UpdateBoardConfiguration atomically applies one board patch.
	UpdateBoardConfiguration(
		context.Context,
		board.ID,
		Patch,
	) (Overrides, error)
}

// Invocation carries normalized configuration mutation attribution.
type Invocation struct{ actor string }

// NewInvocation normalizes configuration mutation attribution.
func NewInvocation(actor string) Invocation {
	return Invocation{actor: strings.TrimSpace(actor)}
}

// Actor returns the normalized mutation actor.
func (i Invocation) Actor() string { return i.actor }

// UpdateRequest selects one configuration layer and finite patch.
type UpdateRequest struct {
	// BoardID identifies the selected board and its containing project.
	BoardID board.ID

	// Scope selects store, project, or board mutation ownership.
	Scope Scope

	// Patch selects values to set or clear.
	Patch Patch
}

// ProjectUpdateRequest identifies one project layer and finite patch without
// requiring a board context.
type ProjectUpdateRequest struct {
	// ProjectID identifies the project that owns the mutable layer.
	ProjectID project.ID

	// Patch selects values to set or clear.
	Patch Patch
}

// Service owns typed configuration resolution and mutation operations.
type Service struct {
	store      Store      // required
	repository Repository // required
	storeID    string
}

// NewService constructs configuration operations for one physical store.
func NewService(store Store, repository Repository) *Service {
	must.NotBeNilf(store, "configuration Store is required")
	must.NotBeNilf(repository, "configuration Repository is required")
	return &Service{store: store, repository: repository}
}

// SetStoreIdentity records the physical store identity reported in views.
func (s *Service) SetStoreIdentity(identity string) {
	s.storeID = identity
}

// Resolve rereads every layer and resolves one board's effective configuration.
func (s *Service) Resolve(ctx context.Context, boardID board.ID) (View, error) {
	storeOverrides, err := s.store.ReadStoreConfiguration(ctx)
	if err != nil {
		return View{}, fmt.Errorf("read store configuration: %w", err)
	}
	layers, err := s.repository.ReadConfigurationLayers(ctx, boardID)
	if err != nil {
		return View{}, fmt.Errorf("read database configuration: %w", err)
	}
	if err := storeOverrides.Validate(); err != nil {
		return View{}, fmt.Errorf("store configuration: %w", err)
	}
	if err := layers.Project.Validate(); err != nil {
		return View{}, fmt.Errorf("project configuration: %w", err)
	}
	if err := layers.Board.Validate(); err != nil {
		return View{}, fmt.Errorf("board configuration: %w", err)
	}
	return resolve(
		s.storeID,
		layers.ProjectID,
		boardID,
		storeOverrides,
		layers.Project,
		layers.Board,
	), nil
}

// ResolveProject rereads and resolves configuration through one project layer.
// It does not select a board or introduce a board layer into the result.
func (s *Service) ResolveProject(
	ctx context.Context,
	projectID project.ID,
) (ProjectView, error) {
	storeOverrides, err := s.store.ReadStoreConfiguration(ctx)
	if err != nil {
		return ProjectView{}, fmt.Errorf("read store configuration: %w", err)
	}
	projectOverrides, err := s.repository.ReadProjectConfiguration(ctx, projectID)
	if err != nil {
		return ProjectView{}, fmt.Errorf("read project configuration: %w", err)
	}
	if err := storeOverrides.Validate(); err != nil {
		return ProjectView{}, fmt.Errorf("store configuration: %w", err)
	}
	if err := projectOverrides.Validate(); err != nil {
		return ProjectView{}, fmt.Errorf("project configuration: %w", err)
	}
	builtIn := Defaults()
	storeLayer := Layer{
		Source:    Source{Scope: ScopeStore, Identity: s.storeID},
		Overrides: storeOverrides,
	}
	projectLayer := Layer{
		Source:    Source{Scope: ScopeProject, Identity: projectID.String()},
		Overrides: projectOverrides,
	}
	effective, origins := resolveLayers(builtIn, storeLayer, projectLayer)
	return ProjectView{
		BuiltIn:   builtIn,
		Store:     storeLayer,
		Project:   projectLayer,
		Effective: effective,
		Origins:   origins,
	}, nil
}

// ResolveConfiguration returns the fully resolved configuration for one board.
func (s *Service) ResolveConfiguration(
	ctx context.Context,
	boardID board.ID,
) (Configuration, error) {
	view, err := s.Resolve(ctx, boardID)
	return view.Effective, err
}

// Update atomically applies one finite layer patch and returns the resulting view.
func (s *Service) Update(
	ctx context.Context,
	_ Invocation,
	request UpdateRequest,
) (View, error) {
	if _, err := board.NewID(request.BoardID.String()); err != nil {
		return View{}, err
	}
	if err := request.Patch.Validate(); err != nil {
		return View{}, err
	}
	switch request.Scope {
	case ScopeStore:
		if _, err := s.store.UpdateStoreConfiguration(ctx, request.Patch); err != nil {
			return View{}, fmt.Errorf("update store configuration: %w", err)
		}
	case ScopeProject:
		layers, err := s.repository.ReadConfigurationLayers(ctx, request.BoardID)
		if err != nil {
			return View{}, fmt.Errorf("select configuration project: %w", err)
		}
		if _, err := s.repository.UpdateProjectConfiguration(
			ctx,
			layers.ProjectID,
			request.Patch,
		); err != nil {
			return View{}, fmt.Errorf("update project configuration: %w", err)
		}
	case ScopeBoard:
		if _, err := s.repository.UpdateBoardConfiguration(
			ctx,
			request.BoardID,
			request.Patch,
		); err != nil {
			return View{}, fmt.Errorf("update board configuration: %w", err)
		}
	default:
		return View{}, fmt.Errorf("configuration scope %q is not mutable", request.Scope.String())
	}
	return s.Resolve(ctx, request.BoardID)
}

// UpdateProject atomically applies one project patch and resolves the
// resulting configuration through that project layer.
// A missing project returns NotFound, and a rejected patch leaves the stored
// layer unchanged.
func (s *Service) UpdateProject(
	ctx context.Context,
	_ Invocation,
	request ProjectUpdateRequest,
) (ProjectView, error) {
	if _, err := project.NewID(request.ProjectID.String()); err != nil {
		return ProjectView{}, err
	}
	if err := request.Patch.Validate(); err != nil {
		return ProjectView{}, err
	}
	if _, err := s.repository.UpdateProjectConfiguration(
		ctx,
		request.ProjectID,
		request.Patch,
	); err != nil {
		return ProjectView{}, fmt.Errorf("update project configuration: %w", err)
	}
	return s.ResolveProject(ctx, request.ProjectID)
}

func resolve(
	storeID string,
	projectID project.ID,
	boardID board.ID,
	store Overrides,
	projectOverrides Overrides,
	boardOverrides Overrides,
) View {
	builtIn := Defaults()
	view := View{
		BuiltIn: builtIn,
		Store: Layer{
			Source: Source{Scope: ScopeStore, Identity: storeID}, Overrides: store,
		},
		Project: Layer{
			Source: Source{Scope: ScopeProject, Identity: projectID.String()}, Overrides: projectOverrides,
		},
		Board: Layer{
			Source: Source{Scope: ScopeBoard, Identity: boardID.String()}, Overrides: boardOverrides,
		},
	}
	view.Effective, view.Origins = resolveLayers(
		builtIn, view.Store, view.Project, view.Board,
	)
	return view
}

// resolveLayers applies layers in the supplied precedence order.
// Each later override replaces both the effective field and its recorded
// origin, so callers must pass layers from least to most specific.
func resolveLayers(
	builtIn Configuration,
	layers ...Layer,
) (Configuration, Origins) {
	builtInSource := Source{Scope: ScopeBuiltIn, Identity: "built-in"}
	effective := builtIn
	origins := Origins{
		Issue: IssueOrigins{
			ID:      IssueIDOrigins{Prefix: builtInSource, Strategy: builtInSource},
			Summary: SummaryOrigins{MaxBytes: builtInSource},
		},
		Attachment: AttachmentOrigins{MaxBytes: builtInSource},
	}
	for _, layer := range layers {
		applyLayer(&effective, &origins, layer)
	}
	return effective, origins
}

func applyLayer(
	effective *Configuration,
	origins *Origins,
	layer Layer,
) {
	if value := layer.Overrides.Issue.ID.Prefix; value != nil {
		effective.Issue.ID.Prefix = *value
		origins.Issue.ID.Prefix = layer.Source
	}
	if value := layer.Overrides.Issue.ID.Strategy; value != nil {
		effective.Issue.ID.Strategy = *value
		origins.Issue.ID.Strategy = layer.Source
	}
	if value := layer.Overrides.Issue.Summary.MaxBytes; value != nil {
		effective.Issue.Summary.MaxBytes = *value
		origins.Issue.Summary.MaxBytes = layer.Source
	}
	if value := layer.Overrides.Attachment.MaxBytes; value != nil {
		effective.Attachment.MaxBytes = *value
		origins.Attachment.MaxBytes = layer.Source
	}
}
