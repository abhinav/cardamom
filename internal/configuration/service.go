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

func resolve(
	storeID string,
	projectID project.ID,
	boardID board.ID,
	store Overrides,
	projectOverrides Overrides,
	boardOverrides Overrides,
) View {
	builtIn := Defaults()
	builtInSource := Source{Scope: ScopeBuiltIn, Identity: "built-in"}
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
		Effective: builtIn,
		Origins: Origins{
			Issue: IssueOrigins{
				ID:      IssueIDOrigins{Prefix: builtInSource, Strategy: builtInSource},
				Summary: SummaryOrigins{MaxBytes: builtInSource},
			},
			Attachment: AttachmentOrigins{MaxBytes: builtInSource},
		},
	}
	applyLayer(&view, view.Store)
	applyLayer(&view, view.Project)
	applyLayer(&view, view.Board)
	return view
}

func applyLayer(view *View, layer Layer) {
	if value := layer.Overrides.Issue.ID.Prefix; value != nil {
		view.Effective.Issue.ID.Prefix = *value
		view.Origins.Issue.ID.Prefix = layer.Source
	}
	if value := layer.Overrides.Issue.ID.Strategy; value != nil {
		view.Effective.Issue.ID.Strategy = *value
		view.Origins.Issue.ID.Strategy = layer.Source
	}
	if value := layer.Overrides.Issue.Summary.MaxBytes; value != nil {
		view.Effective.Issue.Summary.MaxBytes = *value
		view.Origins.Issue.Summary.MaxBytes = layer.Source
	}
	if value := layer.Overrides.Attachment.MaxBytes; value != nil {
		view.Effective.Attachment.MaxBytes = *value
		view.Origins.Attachment.MaxBytes = layer.Source
	}
}
