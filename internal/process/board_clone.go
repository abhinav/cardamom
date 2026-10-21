package process

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration/yamlstore"
	"go.abhg.dev/cardamom/internal/project"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/storelocation"
)

type boardCloneOperation struct {
	config *Config
	source *namespaceRuntime
	board  *board.State
}

func provideBoardCloneOperation(config *Config, source *namespaceRuntime, selected *board.State) cli.BoardCloneOperation {
	return &boardCloneOperation{config: config, source: source, board: selected}
}

var _ cli.BoardCloneOperation = (*boardCloneOperation)(nil)

func (o *boardCloneOperation) Clone(ctx context.Context, request cli.BoardCloneRequest) (out boardclone.Result, err error) {
	directory, err := storelocation.Resolve(request.DestinationStore, o.config.CWD)
	if err != nil {
		return out, err
	}
	same, err := storelocation.SamePhysicalStore(o.source.directory, directory)
	if err != nil {
		return out, err
	}
	if same {
		return out, errors.New("source and destination stores are the same physical store")
	}
	destination, err := openExistingNamespace(ctx, *o.config, directory)
	if err != nil {
		return out, fmt.Errorf("open destination store: %w", err)
	}
	defer func() { err = errors.Join(err, destination.close()) }()
	var selector *project.Selector
	if request.DestinationProject != "" {
		parsed, err := project.NewSelector(request.DestinationProject)
		if err != nil {
			return out, err
		}
		selector = &parsed
	}
	selected, err := destination.projects.Resolve(ctx, selector)
	if err != nil {
		return out, err
	}
	sourceBlobs, err := o.source.attachmentRepository()
	if err != nil {
		return out, err
	}
	destinationBlobs, err := destination.attachmentRepository()
	if err != nil {
		return out, err
	}
	sourceConfiguration, err := (&yamlstore.Store{Directory: o.source.directory}).ReadStoreConfiguration(ctx)
	if err != nil {
		return out, fmt.Errorf("read source store configuration: %w", err)
	}
	destinationConfiguration, err := (&yamlstore.Store{Directory: directory}).ReadStoreConfiguration(ctx)
	if err != nil {
		return out, fmt.Errorf("read destination store configuration: %w", err)
	}
	repository := &repositoryboard.CloneRepository{
		Source: o.source.store, Destination: destination.store,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs,
		Entropy:             destination.entropy,
		SourceConfiguration: sourceConfiguration, DestinationConfiguration: destinationConfiguration,
	}
	return repository.Clone(ctx, boardclone.Request{BoardID: o.board.ID(), ProjectID: selected.ID()})
}
