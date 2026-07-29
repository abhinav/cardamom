package process

import (
	"context"
	"errors"
	"io"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/project"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
	"go.abhg.dev/cardamom/internal/storelocation"
)

// namespaceRuntime owns one open store and the process services that select
// and operate on project boards in that store.
type namespaceRuntime struct {
	directory     string
	store         *store.Store
	configuration *configuration.Service
	catalog       *repositoryproject.Repository
	projects      *project.Service
	boards        *board.Service
	locator       *repositoryboard.Locator
	selection     *selection.Resolver
	clock         Clock
	entropy       io.Reader
}

// openNamespace resolves one physical store and composes its project namespace.
func openNamespace(
	ctx context.Context,
	cfg Config,
	storeSelector string,
) (*namespaceRuntime, error) {
	directory, err := storelocation.Resolve(storeSelector, cfg.CWD)
	if err != nil {
		return nil, err
	}
	persistence, err := store.Open(ctx, store.Config{
		Path: storelocation.DatabasePath(directory),
	})
	if err != nil {
		return nil, err
	}
	return composeNamespace(cfg, directory, persistence)
}

// openExistingNamespace composes services only after the store package has
// verified an initialized Cardamom database.
func openExistingNamespace(
	ctx context.Context,
	cfg Config,
	storeSelector string,
) (*namespaceRuntime, error) {
	directory, err := storelocation.Resolve(storeSelector, cfg.CWD)
	if err != nil {
		return nil, err
	}
	persistence, err := store.OpenExisting(ctx, store.Config{
		Path: storelocation.DatabasePath(directory),
	})
	if err != nil {
		return nil, err
	}
	return composeNamespace(cfg, directory, persistence)
}

func composeNamespace(
	cfg Config,
	directory string,
	persistence *store.Store,
) (*namespaceRuntime, error) {
	catalog := repositoryproject.New(persistence, repositoryproject.Config{
		Clock: cfg.Clock, IDSource: cfg.ProjectIDs,
	})
	configurationService := configuration.NewService(
		settingsStore{directory: directory},
		catalog,
	)
	configurationService.SetStoreIdentity(directory)
	projects := project.NewService(catalog)
	boards := board.NewService(catalog, catalog)
	locator := repositoryboard.NewLocator(persistence)
	bindingPath, err := storelocation.BoardBindingPath(cfg.CWD)
	if err != nil {
		return nil, errors.Join(err, persistence.Close())
	}
	return &namespaceRuntime{
		directory:     directory,
		store:         persistence,
		configuration: configurationService,
		catalog:       catalog,
		projects:      projects,
		boards:        boards,
		locator:       locator,
		selection: selection.NewResolver(
			boards,
			&checkoutBoardBinding{path: bindingPath},
			locator,
		),
		clock:   cfg.Clock,
		entropy: cfg.Entropy,
	}, nil
}

func (r *namespaceRuntime) close() error { return r.store.Close() }

// selectBoard applies invocation-level board and issue selectors.
func (r *namespaceRuntime) selectBoard(
	ctx context.Context,
	selectorValue string,
	issueIDs []string,
) (*board.State, error) {
	var selector *board.Selector
	if selectorValue != "" {
		parsed, err := board.NewSelector(selectorValue)
		if err != nil {
			return nil, err
		}
		selector = &parsed
	}
	return r.selection.Resolve(ctx, selection.Request{
		Selector: selector,
		IssueIDs: issueIDs,
	})
}

// boardRepository applies namespace identity policy to one board repository.
func (r *namespaceRuntime) boardRepository(
	boardID board.ID,
) (*repositoryboard.Repository, error) {
	return repositoryboard.New(r.store, repositoryboard.Config{
		BoardID: boardID, Clock: r.clock,
		Entropy: r.entropy,
	})
}

// issuePlanner constructs planning operations for one board in this namespace.
func (r *namespaceRuntime) issuePlanner(
	boardID board.ID,
) (*planning.Planner, error) {
	repository, err := r.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return planning.NewPlanner(repository, repository, &planning.PlannerOptions{
		BoardID: boardID, Configuration: r.configuration,
	}), nil
}

// issueQueries constructs finite issue reads for one board in this namespace.
func (r *namespaceRuntime) issueQueries(
	boardID board.ID,
) (*issue.Queries, error) {
	repository, err := r.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return issue.NewQueries(repository), nil
}

// issueExecutor constructs execution operations for one board in this namespace.
func (r *namespaceRuntime) issueExecutor(
	boardID board.ID,
) (*execution.Executor, error) {
	repository, err := r.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return execution.NewExecutor(repository, repository), nil
}

// issueRecorder constructs record operations for one board in this namespace.
func (r *namespaceRuntime) issueRecorder(
	boardID board.ID,
) (*record.Recorder, error) {
	repository, err := r.boardRepository(boardID)
	if err != nil {
		return nil, err
	}
	return record.NewRecorder(repository, repository), nil
}

func provideNamespace(
	invocation *cli.Invocation,
	config *Config,
	cleanup *selectedNamespaceCleanup,
) (*namespaceRuntime, error) {
	runtime, err := openNamespace(invocation.Context, *config, invocation.Store)
	if err != nil {
		return nil, err
	}
	cleanup.register(runtime)
	return runtime, nil
}

func provideProjectService(
	runtime *namespaceRuntime,
) *project.Service {
	return runtime.projects
}

func provideConfigurationService(
	runtime *namespaceRuntime,
) *configuration.Service {
	return runtime.configuration
}

func provideBoardResolver(runtime *namespaceRuntime) *selection.Resolver {
	return runtime.selection
}

func provideBoardService(
	runtime *namespaceRuntime,
) *board.Service {
	return runtime.boards
}

func provideBoardCatalog(
	boards *board.Service,
) cli.BoardCatalog {
	return boards
}

func provideSelectedBoard(
	invocation *cli.Invocation,
	runtime *namespaceRuntime,
) (*board.State, error) {
	selected, err := runtime.selectBoard(
		invocation.Context,
		invocation.Board,
		invocation.BoardIssueIDs,
	)
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func provideBoardRepository(
	runtime *namespaceRuntime,
	selected *board.State,
) (*repositoryboard.Repository, error) {
	return runtime.boardRepository(selected.ID())
}
