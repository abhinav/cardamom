package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/project"
)

// PreparedRestore contains one fully verified archive ready for destination
// validation and application.
//
// The source backing the Reader must remain readable until Restore completes.
// Board snapshots are materialized during preparation, while verified blob
// bodies are reopened and streamed during application.
type PreparedRestore struct {
	reader   *Reader
	projects []project.Snapshot
	boards   []preparedRestoreBoard
	blobs    []attachment.BlobDescriptor
}

// preparedRestoreBoard keeps one verified snapshot associated with the
// publication metadata needed during destination application.
type preparedRestoreBoard struct {
	publication Board
	snapshot    boardcopy.CopySnapshot
}

// PrepareRestore reads and verifies every archived board and blob without
// consulting or mutating a destination.
func PrepareRestore(
	ctx context.Context,
	reader *Reader,
) (*PreparedRestore, error) {
	if reader == nil {
		return nil, errors.New("backup reader is required")
	}

	descriptors := reader.Blobs()
	indexedBlobs := make(map[attachment.Digest]attachment.BlobDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		indexedBlobs[descriptor.Digest] = descriptor
	}

	publications := reader.Boards()
	boards := make([]preparedRestoreBoard, 0, len(publications))
	for _, publication := range publications {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapshot, err := reader.ReadBoard(publication)
		if err != nil {
			return nil, err
		}
		for _, value := range snapshot.Attachments {
			indexed, found := indexedBlobs[value.Blob.Digest]
			if !found || indexed.SizeBytes != value.Blob.SizeBytes {
				return nil, fmt.Errorf(
					"archive board %q attachment %q references unindexed blob %s",
					publication.SourceBoardID,
					value.ID,
					value.Blob.Digest,
				)
			}
		}
		boards = append(boards, preparedRestoreBoard{
			publication: publication,
			snapshot:    snapshot,
		})
	}

	for _, descriptor := range descriptors {
		if err := verifyArchiveBlob(ctx, reader, descriptor); err != nil {
			return nil, err
		}
	}
	return &PreparedRestore{
		reader: reader, projects: reader.Projects(), boards: boards,
		blobs: descriptors,
	}, nil
}

// ProjectDestination preflights and reconciles archived project identity.
type ProjectDestination interface {
	// ValidateRestoreProjects checks retained metadata without mutation.
	ValidateRestoreProjects(context.Context, []project.Snapshot) error

	// RestoreProjects atomically rechecks retained metadata and creates every
	// missing project with its archived identity.
	RestoreProjects(context.Context, []project.Snapshot) error
}

// RestoreServiceConfig supplies destination persistence for one restore.
type RestoreServiceConfig struct {
	Projects ProjectDestination                // required
	Boards   boardcopy.CopySnapshotDestination // required
	Blobs    boardcopy.CopyBlobDestination     // required
}

// RestoreService loads complete portable backups into an existing store.
type RestoreService struct {
	projects ProjectDestination
	boards   boardcopy.CopySnapshotDestination
	blobs    boardcopy.CopyBlobDestination
}

// NewRestoreService constructs the portable backup restore operation.
func NewRestoreService(cfg RestoreServiceConfig) *RestoreService {
	must.NotBeNilf(cfg.Projects, "backup restore projects are required")
	must.NotBeNilf(cfg.Boards, "backup restore boards are required")
	must.NotBeNilf(cfg.Blobs, "backup restore blobs are required")
	return &RestoreService{
		projects: cfg.Projects,
		boards:   cfg.Boards,
		blobs:    cfg.Blobs,
	}
}

// RestoreResult reports every archived project and imported board.
type RestoreResult struct {
	// Projects contains the archived project metadata in manifest order.
	Projects []project.Snapshot

	// Boards reports new and previously completed board imports.
	Boards []boardcopy.CopyOutcome

	// BlobCount is the number of deduplicated blobs published from the archive.
	BlobCount int
}

// Restore validates destination compatibility before applying one prepared
// archive, then imports each board as an independently restartable operation.
func (s *RestoreService) Restore(
	ctx context.Context,
	prepared *PreparedRestore,
) (RestoreResult, error) {
	if prepared == nil || prepared.reader == nil {
		return RestoreResult{}, errors.New("prepared backup restore is required")
	}

	projects := prepared.projects
	if err := s.projects.ValidateRestoreProjects(ctx, projects); err != nil {
		return RestoreResult{}, fmt.Errorf("validate destination projects: %w", err)
	}
	if err := s.projects.RestoreProjects(ctx, projects); err != nil {
		return RestoreResult{}, fmt.Errorf("restore projects: %w", err)
	}
	for _, descriptor := range prepared.blobs {
		if err := s.publishBlob(ctx, prepared.reader, descriptor); err != nil {
			return RestoreResult{}, err
		}
	}

	result := RestoreResult{
		Projects:  slices.Clone(projects),
		Boards:    make([]boardcopy.CopyOutcome, 0, len(prepared.boards)),
		BlobCount: len(prepared.blobs),
	}
	for _, board := range prepared.boards {
		outcome, err := s.restoreBoard(
			ctx,
			board.snapshot,
			board.publication.ProjectID,
		)
		if err != nil {
			return RestoreResult{}, fmt.Errorf(
				"restore board %q: %w",
				board.publication.SourceBoardID,
				err,
			)
		}
		result.Boards = append(result.Boards, outcome)
	}
	return result, nil
}

func verifyArchiveBlob(
	ctx context.Context,
	reader *Reader,
	descriptor attachment.BlobDescriptor,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := reader.OpenBlob(descriptor)
	if err != nil {
		return fmt.Errorf("open archive blob %s: %w", descriptor.Digest, err)
	}
	_, readErr := io.Copy(io.Discard, source)
	err = errors.Join(readErr, source.Close())
	if err != nil {
		return fmt.Errorf("verify archive blob %s: %w", descriptor.Digest, err)
	}
	return nil
}

func (s *RestoreService) publishBlob(
	ctx context.Context,
	reader *Reader,
	descriptor attachment.BlobDescriptor,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := reader.OpenBlob(descriptor)
	if err != nil {
		return fmt.Errorf("open archive blob %s: %w", descriptor.Digest, err)
	}
	publishErr := s.blobs.PublishCopyBlob(ctx, descriptor, source)
	err = errors.Join(publishErr, source.Close())
	if err != nil {
		return fmt.Errorf("publish archive blob %s: %w", descriptor.Digest, err)
	}
	return nil
}

func (s *RestoreService) restoreBoard(
	ctx context.Context,
	snapshot boardcopy.CopySnapshot,
	projectID project.ID,
) (boardcopy.CopyOutcome, error) {
	options := boardcopy.CopyOptions{ProjectID: projectID.String()}
	receipt, found, err := s.boards.ReadCopyReceipt(
		ctx,
		boardcopy.CopyReceiptKey{
			SourceLineageID: snapshot.SourceLineageID,
			SourceBoardID:   snapshot.Board.ID,
			SnapshotVersion: snapshot.Version,
		},
	)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	if found {
		outcome, err := boardcopy.EvaluateCopyReceipt(snapshot, options, receipt)
		if err != nil {
			return boardcopy.CopyOutcome{}, err
		}
		outcome.Counts.Blobs = snapshotBlobCount(snapshot)
		return outcome, nil
	}

	imported, err := s.boards.ImportCopySnapshot(ctx, snapshot, options)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	var outcome boardcopy.CopyOutcome
	switch {
	case imported.Published != nil && imported.Competing == nil:
		outcome = *imported.Published
	case imported.Published == nil && imported.Competing != nil:
		outcome, err = boardcopy.EvaluateCopyReceipt(
			snapshot,
			options,
			*imported.Competing,
		)
		if err != nil {
			return boardcopy.CopyOutcome{}, err
		}
	default:
		return boardcopy.CopyOutcome{}, errors.New(
			"destination board copy returned an invalid import result",
		)
	}
	outcome.Counts.Blobs = snapshotBlobCount(snapshot)
	return outcome, nil
}

func snapshotBlobCount(snapshot boardcopy.CopySnapshot) int {
	digests := make(map[attachment.Digest]struct{}, len(snapshot.Attachments))
	for _, value := range snapshot.Attachments {
		digests[value.Blob.Digest] = struct{}{}
	}
	return len(digests)
}
