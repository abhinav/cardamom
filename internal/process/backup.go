package process

import (
	"context"
	"errors"
	"fmt"
	"os"

	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
	"go.abhg.dev/cardamom/internal/cli"
	repositoryattachment "go.abhg.dev/cardamom/internal/repository/attachment"
	repositorybackup "go.abhg.dev/cardamom/internal/repository/backup"
	repositoryboard "go.abhg.dev/cardamom/internal/repository/board"
	"go.abhg.dev/cardamom/internal/storelocation"
)

type backupOperation struct {
	config *Config
}

func provideBackupOperation(config *Config) cli.BackupOperation {
	return &backupOperation{config: config}
}

func (o *backupOperation) Backup(
	ctx context.Context,
	request cli.BackupRequest,
) (result cli.BackupResult, err error) {
	runtime, err := openNamespace(ctx, *o.config, request.SourceStore)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, runtime.close()) }()
	selected, err := o.selection(ctx, runtime, request)
	if err != nil {
		return result, err
	}
	destination, err := absolutePath(request.Destination, o.config.CWD)
	if err != nil {
		return result, err
	}
	blobs, err := runtime.attachmentRepository()
	if err != nil {
		return result, err
	}
	operation := domainbackup.NewOperation(domainbackup.OperationConfig{
		Source:        repositorybackup.New(runtime.store),
		Blobs:         blobs,
		Configuration: settingsStore{directory: runtime.directory},
		Publisher:     &domainbackup.FilePublisher{},
	})
	written, err := operation.Write(ctx, domainbackup.WriteRequest{
		Destination: destination,
		Selection:   selected,
	})
	if err != nil {
		return result, err
	}
	return cli.BackupResult{
		Source: runtime.directory, Destination: written.Destination,
		Projects: written.Projects, Boards: written.Boards, Blobs: written.Blobs,
	}, nil
}

func (o *backupOperation) selection(
	ctx context.Context,
	runtime *namespaceRuntime,
	request cli.BackupRequest,
) (domainbackup.Selection, error) {
	if request.All {
		return domainbackup.AllBoards(), nil
	}
	selectors := request.IncludeBoards
	if len(selectors) == 0 {
		selected, err := runtime.selectBoard(ctx, request.DefaultBoard, nil)
		if err != nil {
			return domainbackup.Selection{}, err
		}
		return domainbackup.SelectBoards(selected.ID())
	}

	boardIDs := make([]board.ID, 0, len(selectors))
	for _, value := range selectors {
		selector, err := board.NewSelector(value)
		if err != nil {
			return domainbackup.Selection{}, err
		}
		selected, err := runtime.selection.Resolve(ctx, selection.Request{
			Selector: &selector,
		})
		if err != nil {
			return domainbackup.Selection{}, err
		}
		boardIDs = append(boardIDs, selected.ID())
	}
	return domainbackup.SelectBoards(boardIDs...)
}

type restoreOperation struct{ config *Config }

func provideRestoreOperation(config *Config) cli.RestoreOperation {
	return &restoreOperation{config: config}
}

func (o *restoreOperation) Restore(
	ctx context.Context,
	request cli.RestoreRequest,
) (result cli.RestoreResult, err error) {
	source, err := absolutePath(request.Source, o.config.CWD)
	if err != nil {
		return result, err
	}
	archive, err := os.Open(source)
	if err != nil {
		return result, fmt.Errorf("open backup archive %q: %w", source, err)
	}
	defer func() { err = errors.Join(err, archive.Close()) }()
	info, err := archive.Stat()
	if err != nil {
		return result, fmt.Errorf("inspect backup archive %q: %w", source, err)
	}
	reader, err := domainbackup.NewReader(archive, info.Size())
	if err != nil {
		return result, fmt.Errorf("read backup archive %q: %w", source, err)
	}
	prepared, err := domainbackup.PrepareRestore(ctx, reader)
	if err != nil {
		return result, fmt.Errorf("prepare backup restore: %w", err)
	}

	destinationDirectory, err := o.destinationDirectory(request)
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(destinationDirectory, 0o755); err != nil {
		return result, fmt.Errorf(
			"create destination store directory %q: %w",
			destinationDirectory,
			err,
		)
	}
	destination, err := openNamespace(ctx, *o.config, destinationDirectory)
	if err != nil {
		return result, fmt.Errorf("open destination store: %w", err)
	}
	defer func() { err = errors.Join(err, destination.close()) }()

	boards, err := repositoryboard.NewCopyRepository(
		destination.store,
		repositoryboard.CopyRepositoryConfig{
			Clock: destination.clock, Entropy: destination.entropy,
		},
	)
	if err != nil {
		return result, err
	}
	blobs, err := repositoryattachment.New(
		destination.store,
		repositoryattachment.Config{
			StoreDirectory: destination.directory,
			Clock:          destination.clock,
			Entropy:        destination.entropy,
		},
	)
	if err != nil {
		return result, err
	}
	restored, err := domainbackup.NewRestoreService(
		domainbackup.RestoreServiceConfig{
			Projects: destination.catalog,
			Boards:   boards,
			Blobs:    blobs,
		},
	).Restore(ctx, prepared)
	if err != nil {
		return result, err
	}
	alreadyCompleted := 0
	for _, board := range restored.Boards {
		if board.AlreadyCompleted {
			alreadyCompleted++
		}
	}
	return cli.RestoreResult{
		Source: source, Destination: destination.directory,
		Projects: len(restored.Projects), Boards: len(restored.Boards),
		Blobs:                  restored.BlobCount,
		AlreadyCompletedBoards: alreadyCompleted,
	}, nil
}

func (o *restoreOperation) destinationDirectory(
	request cli.RestoreRequest,
) (string, error) {
	if !request.DestinationStoreExplicit {
		return storelocation.Resolve(request.DestinationStore, o.config.CWD)
	}
	target, err := absolutePath(request.DestinationStore, o.config.CWD)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
		return target, nil
	} else if err != nil {
		return "", fmt.Errorf("inspect destination store %q: %w", target, err)
	}
	return storelocation.Resolve(request.DestinationStore, o.config.CWD)
}
