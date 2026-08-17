package process

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/cli"
	"go.abhg.dev/cardamom/internal/configuration/yamlstore"
	"go.abhg.dev/cardamom/internal/project"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/storelocation"
)

type boardCopyOperation struct {
	config *Config
	source *namespaceRuntime
	board  *board.State
}

func provideBoardCopyOperation(
	config *Config,
	source *namespaceRuntime,
	selected *board.State,
) cli.BoardCopyOperation {
	return &boardCopyOperation{
		config: config, source: source, board: selected,
	}
}

func (o *boardCopyOperation) Copy(
	ctx context.Context,
	request cli.BoardCopyRequest,
) (out boardcopy.CopyOutcome, err error) {
	destinationDirectory, err := storelocation.Resolve(
		request.DestinationStore,
		o.config.CWD,
	)
	if err != nil {
		return out, err
	}
	same, err := storelocation.SamePhysicalStore(
		o.source.directory,
		destinationDirectory,
	)
	if err != nil {
		return out, err
	}
	if same {
		return out, errors.New(
			"source and destination stores are the same physical store",
		)
	}

	destination, err := openExistingNamespace(
		ctx,
		*o.config,
		destinationDirectory,
	)
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
	destinationProject, err := destination.projects.Resolve(ctx, selector)
	if err != nil {
		return out, err
	}

	sourceRepository, err := o.source.boardRepository(o.board.ID())
	if err != nil {
		return out, err
	}
	sourceAttachments, err := o.source.attachmentRepository()
	if err != nil {
		return out, err
	}
	destinationRepository, err := repositoryboard.NewCopyRepository(
		destination.store,
		repositoryboard.CopyRepositoryConfig{
			Clock: destination.clock, Entropy: destination.entropy,
		},
	)
	if err != nil {
		return out, err
	}
	destinationAttachments, err := destination.attachmentRepository()
	if err != nil {
		return out, err
	}
	service := boardcopy.NewCopyService(boardcopy.CopyServiceConfig{
		Source:           sourceRepository,
		Destination:      destinationRepository,
		SourceBlobs:      sourceAttachments,
		DestinationBlobs: destinationAttachments,
		Configuration:    &yamlstore.Store{Directory: o.source.directory},
	})
	return service.Copy(ctx, boardcopy.CopyRequest{
		SourceBoardID: o.board.ID(),
		Options: boardcopy.CopyOptions{
			ProjectID: destinationProject.ID().String(),
			Name:      request.Name,
		},
	})
}
