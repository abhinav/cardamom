package board

import (
	"context"
	"strings"

	"go.abhg.dev/cardamom/internal/must"
)

// Invocation carries attribution resolved for one command or HTTP request.
// It is immutable invocation state rather than service configuration.
type Invocation struct {
	// actor is normalized attribution for one caller invocation.
	actor string
}

// NewInvocation normalizes board mutation attribution supplied at a process boundary.
func NewInvocation(actor string) Invocation {
	return Invocation{actor: strings.TrimSpace(actor)}
}

// Actor returns the normalized invocation actor.
func (i Invocation) Actor() string { return i.actor }

// CreateRequest supplies the state required to establish a board.
type CreateRequest struct {
	// ProjectID identifies the project that will contain the board.
	ProjectID string

	// Name is the user-visible board name.
	Name string

	// Description is the optional initial Markdown shared by the board.
	Description *string
}

// EditRequest selects settings changed on one board.
type EditRequest struct {
	// BoardID identifies the board to edit.
	BoardID ID

	// Settings contains the atomic name and description edit.
	Settings SettingsEdit
}

// ArchiveRequest selects a board and optional reason for archival.
type ArchiveRequest struct {
	// BoardID identifies the board to archive.
	BoardID ID

	// Reason is an optional non-empty explanation for the transition.
	Reason *string
}

// ArchiveResult reports the committed lifecycle state and issue population.
type ArchiveResult struct {
	// Board is the committed lifecycle state.
	Board *State

	// Changed reports whether this invocation performed the archive transition.
	Changed bool

	// Issues is the effective status population observed by the archive writer.
	Issues IssueCounts
}

// IssueCounts reports the effective status population observed in the same
// transaction as an archive attempt. The status fields partition Total.
type IssueCounts struct {
	// Total is the number of issues in the board.
	Total int

	// Ready is the number of issues whose effective status is ready.
	Ready int

	// Blocked is the number of issues whose effective status is blocked.
	Blocked int

	// InProgress is the number of issues whose effective status is in progress.
	InProgress int

	// Waiting is the number of issues whose effective status is waiting.
	Waiting int

	// Closed is the number of issues whose effective status is closed.
	Closed int

	// Cancelled is the number of issues whose effective status is cancelled.
	Cancelled int
}

//go:generate go tool mockgen -destination mocks_test.go -package board -typed -write_package_comment=false . Catalog,Changes

// Catalog reads boards in deterministic catalog order and by stable identity.
type Catalog interface {
	// ListAllBoards returns every board in deterministic order.
	ListAllBoards(context.Context) ([]*State, error)

	// Board returns one board by stable identity.
	Board(context.Context, ID) (*State, error)

	// SoleBoard returns the only board or an ambiguity error.
	SoleBoard(context.Context) (*State, error)
}

// Changes persists finite board mutations and returns committed board state.
type Changes interface {
	// CreateBoard atomically establishes one board.
	CreateBoard(context.Context, CreateRequest) (*State, error)

	// EditBoardSettings atomically changes one board's settings.
	EditBoardSettings(context.Context, EditRequest) (*State, error)

	// ArchiveBoard atomically archives a board without active custody and
	// reports the issue population observed by that writer.
	ArchiveBoard(context.Context, Invocation, ArchiveRequest) (ArchiveResult, error)

	// UnarchiveBoard atomically clears archive metadata and reports whether the
	// lifecycle state changed.
	UnarchiveBoard(context.Context, ID) (*State, bool, error)
}

// Archive logically archives one board when it has no active claim. An already
// archived board succeeds without replacing its metadata; Changed distinguishes
// that no-op from a transition.
func (s *Service) Archive(ctx context.Context, invocation Invocation, request ArchiveRequest) (ArchiveResult, error) {
	return s.changes.ArchiveBoard(ctx, invocation, request)
}

// Unarchive restores one archived board to active mutation service. Changed is
// false when the board was already active.
func (s *Service) Unarchive(ctx context.Context, boardID ID) (*State, bool, error) {
	return s.changes.UnarchiveBoard(ctx, boardID)
}

// Service owns finite board catalog and mutation operations.
type Service struct {
	// catalog supplies board reads.
	catalog Catalog

	// changes owns the atomic persistence boundary for board mutations.
	changes Changes
}

// NewService constructs finite board catalog and mutation operations.
// It panics when a dependency is nil because process composition must provide it.
func NewService(catalog Catalog, changes Changes) *Service {
	must.NotBeNilf(catalog, "board Catalog is required")
	must.NotBeNilf(changes, "board Changes is required")
	return &Service{catalog: catalog, changes: changes}
}

// List returns every board in deterministic catalog order.
func (s *Service) List(ctx context.Context) ([]*State, error) {
	return s.catalog.ListAllBoards(ctx)
}

// Get returns one board by stable identity.
func (s *Service) Get(ctx context.Context, boardID ID) (*State, error) {
	return s.catalog.Board(ctx, boardID)
}

// Sole returns the only board or an ambiguity error.
func (s *Service) Sole(ctx context.Context) (*State, error) {
	return s.catalog.SoleBoard(ctx)
}

// Create establishes one board.
func (s *Service) Create(
	ctx context.Context,
	_ Invocation,
	request CreateRequest,
) (*State, error) {
	return s.changes.CreateBoard(ctx, request)
}

// EditSettings changes one board's settings.
func (s *Service) EditSettings(
	ctx context.Context,
	_ Invocation,
	request EditRequest,
) (*State, error) {
	return s.changes.EditBoardSettings(ctx, request)
}
