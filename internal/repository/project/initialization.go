package project

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
	"go.abhg.dev/cardamom/internal/storelocation"
)

var _ project.StoreInitializer = (*Initializer)(nil)

// Initializer adapts product namespace initialization to the storage
// lifecycle owned by store.Initializer.
type Initializer struct {
	// stores owns fresh database publication and migration lifecycle.
	stores *store.Initializer

	// config supplies project timestamps and identities during initialization.
	config Config
}

// NewInitializer constructs a project store initializer with shared namespace
// dependencies.
func NewInitializer(config Config) *Initializer {
	return &Initializer{
		stores: store.NewInitializer(),
		config: config,
	}
}

// InitializeStore creates or migrates a project store and completes a fresh
// namespace before the database is published.
func (i *Initializer) InitializeStore(
	ctx context.Context,
	request project.StoreInitializationRequest,
) (project.StoreInitialization, error) {
	var namespace project.InitializedNamespace
	initialized, err := i.stores.Initialize(ctx, request.Dir, func(ctx context.Context, persistence *store.Store) error {
		var err error
		namespace, err = initializeFreshProject(
			ctx,
			persistence,
			request,
			i.config,
		)
		return err
	})
	result := project.StoreInitialization{
		DatabaseWritten: initialized.DatabaseWritten,
		SchemaVersion:   int(initialized.SchemaVersion),
	}
	if err != nil {
		return result, err
	}
	if initialized.DatabaseWritten {
		result.Namespace = &namespace
		result.ProjectIDPrefix = request.FreshProjectIDPrefix
		return result, nil
	}
	result.ProjectIDPrefix, err = i.retainedProjectIDPrefix(
		ctx,
		request.Dir,
		request.RetainedProjectIDPrefix,
	)
	if err != nil {
		return result, err
	}
	return result, nil
}

// retainedProjectIDPrefix reads the sole retained project's prefix after
// applying an optional explicit update through configuration mutation.
func (i *Initializer) retainedProjectIDPrefix(
	ctx context.Context,
	directory string,
	requested *string,
) (out *string, err error) {
	persistence, err := store.Open(ctx, store.Config{
		Path: storelocation.DatabasePath(directory),
	})
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, persistence.Close()) }()

	repository := New(persistence, i.config)
	catalog := project.NewService(repository, repository)
	projects, err := catalog.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list retained projects: %w", err)
	}
	if len(projects) != 1 {
		if requested == nil {
			return nil, nil
		}
		_, err := catalog.Resolve(ctx, nil)
		return nil, fmt.Errorf("resolve retained project: %w", err)
	}
	namespace := projects[0]
	if requested != nil {
		prefix, err := configuration.NewPrefix(*requested)
		if err != nil {
			return nil, err
		}
		overrides, err := repository.UpdateProjectConfiguration(
			ctx,
			namespace.ID(),
			configuration.Patch{
				Fields: []configuration.Field{
					configuration.FieldIssueIDPrefix,
				},
				Overrides: configuration.Overrides{
					Issue: configuration.IssueOverrides{
						ID: configuration.IssueIDOverrides{
							Prefix: &prefix,
						},
					},
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"update retained project configuration: %w",
				err,
			)
		}
		return nullablePrefix(overrides.Issue.ID.Prefix), nil
	}
	view, err := persistence.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	row, err := query.New(view).ProjectGetProjectConfiguration(
		ctx,
		namespace.ID().String(),
	)
	if err != nil {
		return nil, fmt.Errorf("read retained project configuration: %w", err)
	}
	if row.IssueIDPrefix == nil {
		return nil, nil
	}
	prefix, err := configuration.NewPrefix(*row.IssueIDPrefix)
	if err != nil {
		return nil, fmt.Errorf("load retained project prefix: %w", err)
	}
	return new(prefix.String()), nil
}

// initializeFreshProject validates and commits the complete initial namespace
// as one logical transaction.
func initializeFreshProject(
	ctx context.Context,
	persistence *store.Store,
	request project.StoreInitializationRequest,
	config Config,
) (project.InitializedNamespace, error) {
	repository := New(persistence, config)
	created := repository.clock()
	projectID, err := repository.idSource.NewID("project")
	if err != nil {
		return project.InitializedNamespace{}, fmt.Errorf(
			"generate project identity: %w",
			err,
		)
	}
	namespace, err := project.Load(project.Snapshot{
		ID:      project.ID(projectID),
		Name:    request.ProjectName,
		Created: created,
	})
	if err != nil {
		return project.InitializedNamespace{}, err
	}

	out := project.InitializedNamespace{Project: namespace}
	if request.BoardName != nil {
		boardID, err := repository.idSource.NewID("board")
		if err != nil {
			return project.InitializedNamespace{}, fmt.Errorf(
				"generate board identity: %w",
				err,
			)
		}
		state, err := board.Load(board.Snapshot{
			ID:        board.ID(boardID),
			ProjectID: namespace.ID().String(),
			Name:      *request.BoardName,
			Created:   created,
		})
		if err != nil {
			return project.InitializedNamespace{}, err
		}
		out.Board = state
	}

	err = repository.commitRevision(ctx, func(change *store.Change) error {
		queries := query.New(change)
		if err := queries.ProjectCreateInitialProject(
			ctx,
			query.ProjectCreateInitialProjectParams{
				ID:            namespace.ID().String(),
				Name:          namespace.Name(),
				CreatedAt:     namespace.Created(),
				IssueIDPrefix: request.FreshProjectIDPrefix,
			},
		); err != nil {
			return fmt.Errorf("create initial project: %w", err)
		}
		if out.Board == nil {
			return nil
		}
		if err := queries.ProjectCreateInitialBoard(
			ctx,
			query.ProjectCreateInitialBoardParams{
				ID:        out.Board.ID().String(),
				ProjectID: out.Board.ProjectID(),
				Name:      out.Board.Name(),
				CreatedAt: out.Board.Created(),
			},
		); err != nil {
			return fmt.Errorf("create initial board: %w", err)
		}
		return nil
	})
	if err != nil {
		return project.InitializedNamespace{}, err
	}
	return out, nil
}
