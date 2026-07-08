package information

import (
	"context"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// Projects resolves the project containing a selected board.
type Projects interface {
	// Resolve selects an explicit project or the store's sole project.
	Resolve(context.Context, *project.Selector) (*project.State, error)
}

// Boards reads a selected coordination board.
type Boards interface {
	// Get returns one board by stable identity.
	Get(context.Context, board.ID) (*board.State, error)
}

// Configurations resolves effective configuration for a selected board.
type Configurations interface {
	// ResolveConfiguration returns the fully resolved configuration.
	ResolveConfiguration(context.Context, board.ID) (configuration.Configuration, error)
}

// Readers creates snapshot readers for selected boards.
type Readers interface {
	// Reader returns a reader scoped to boardID and its effective configuration.
	Reader(board.ID, configuration.Configuration) (Reader, error)
}

// ServiceConfig supplies store identity and collaborators used by Service.
type ServiceConfig struct {
	// Store identifies the physical persistence boundary.
	Store Store

	Projects       Projects       // required
	Boards         Boards         // required
	Configurations Configurations // required
	Readers        Readers        // required
}

// Service composes the typed store information operation.
type Service struct {
	store          Store
	projects       Projects
	boards         Boards
	configurations Configurations
	readers        Readers
}

// NewService constructs store information operations.
func NewService(cfg ServiceConfig) *Service {
	must.NotBeNilf(cfg.Projects, "information Projects is required")
	must.NotBeNilf(cfg.Boards, "information Boards is required")
	must.NotBeNilf(cfg.Configurations, "information Configurations is required")
	must.NotBeNilf(cfg.Readers, "information Readers is required")
	return &Service{
		store: cfg.Store, projects: cfg.Projects, boards: cfg.Boards,
		configurations: cfg.Configurations, readers: cfg.Readers,
	}
}

// Request selects one board for the information projection.
type Request struct {
	// BoardID identifies the selected coordination board.
	BoardID board.ID
}

// Report contains typed store identity and inventory information.
type Report struct {
	// Store identifies the physical persistence boundary.
	Store Store

	// Project identifies the project containing Board.
	Project *project.State

	// Board identifies the selected coordination board.
	Board *board.State

	// Schema identifies persisted and running schema versions.
	Schema Schema

	// Configuration is the selected board's effective configuration.
	Configuration configuration.Configuration

	// Revision identifies the canonical store revision.
	Revision Revision

	// Issues reports the selected board's issue population.
	Issues IssueInventory
}

// Read returns typed identity and inventory for one selected board.
func (s *Service) Read(ctx context.Context, request Request) (Report, error) {
	selectedBoard, err := s.boards.Get(ctx, request.BoardID)
	if err != nil {
		return Report{}, err
	}
	projectSelector, err := project.NewSelector(selectedBoard.ProjectID())
	if err != nil {
		return Report{}, err
	}
	selectedProject, err := s.projects.Resolve(ctx, &projectSelector)
	if err != nil {
		return Report{}, err
	}
	effective, err := s.configurations.ResolveConfiguration(ctx, selectedBoard.ID())
	if err != nil {
		return Report{}, err
	}
	reader, err := s.readers.Reader(selectedBoard.ID(), effective)
	if err != nil {
		return Report{}, err
	}
	snapshot, err := reader.Read(ctx)
	if err != nil {
		return Report{}, err
	}
	return Report{
		Store: s.store, Project: selectedProject, Board: selectedBoard,
		Schema: snapshot.Schema, Configuration: effective,
		Revision: snapshot.Revision, Issues: snapshot.Issues,
	}, nil
}
