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
// Preparation retains compact project, publication, record-index, and blob
// metadata.
// Board members and verified blob bodies are reopened during application.
type PreparedRestore struct {
	reader   *Reader
	projects []project.Snapshot
	boards   []preparedBoard
	blobs    []attachment.BlobDescriptor
}

// preparedBoard binds one manifest publication to its fully verified semantic
// index without retaining its record bodies.
type preparedBoard struct {
	publication Board
	index       boardcopy.RecordIndex
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
	preparedBoards := make([]preparedBoard, 0, len(publications))
	for _, publication := range publications {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		index, err := preflightBoard(ctx, reader, publication)
		if err != nil {
			return nil, err
		}
		for _, descriptor := range index.Blobs {
			indexed, found := indexedBlobs[descriptor.Digest]
			if !found || indexed.SizeBytes != descriptor.SizeBytes {
				return nil, fmt.Errorf(
					"archive board %q references unindexed blob %s",
					publication.SourceBoardID,
					descriptor.Digest,
				)
			}
		}
		preparedBoards = append(preparedBoards, preparedBoard{
			publication: publication,
			index:       index,
		})
	}

	for _, descriptor := range descriptors {
		if err := verifyArchiveBlob(ctx, reader, descriptor); err != nil {
			return nil, err
		}
	}
	return &PreparedRestore{
		reader: reader, projects: reader.Projects(), boards: preparedBoards,
		blobs: descriptors,
	}, nil
}

func preflightBoard(
	ctx context.Context,
	reader *Reader,
	publication Board,
) (boardcopy.RecordIndex, error) {
	records, err := reader.OpenBoard(publication)
	if err != nil {
		return boardcopy.RecordIndex{}, err
	}
	indexer := boardcopy.NewRecordIndexer()
	for record, recordErr := range records {
		if err := ctx.Err(); err != nil {
			return boardcopy.RecordIndex{}, err
		}
		if recordErr != nil {
			return boardcopy.RecordIndex{}, fmt.Errorf(
				"read archive board %q: %w",
				publication.SourceBoardID,
				recordErr,
			)
		}
		if err := indexer.Add(record); err != nil {
			return boardcopy.RecordIndex{}, fmt.Errorf(
				"read archive board %q: %w",
				publication.SourceBoardID,
				err,
			)
		}
	}
	if err := ctx.Err(); err != nil {
		return boardcopy.RecordIndex{}, err
	}
	index, err := indexer.Finish()
	if err != nil {
		return boardcopy.RecordIndex{}, fmt.Errorf(
			"read archive board %q: %w",
			publication.SourceBoardID,
			err,
		)
	}
	header := index.Header
	if header.SourceLineageID != publication.SourceLineageID ||
		header.SourceRevision != publication.SourceRevision ||
		header.Version != publication.SnapshotVersion ||
		header.Board.ID != publication.SourceBoardID.String() {
		return boardcopy.RecordIndex{}, fmt.Errorf(
			"archive board %q does not match its manifest publication",
			publication.SourceBoardID,
		)
	}
	if index.Digest != publication.SnapshotDigest {
		return boardcopy.RecordIndex{}, fmt.Errorf(
			"archive board %q semantic digest does not match its manifest publication",
			publication.SourceBoardID,
		)
	}
	return index, nil
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
	Projects ProjectDestination            // required
	Boards   boardcopy.RecordDestination   // required
	Blobs    boardcopy.CopyBlobDestination // required
}

// RestoreService loads complete portable backups into an existing store.
type RestoreService struct {
	projects ProjectDestination
	boards   boardcopy.RecordDestination
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
	for _, preparedBoard := range prepared.boards {
		outcome, err := s.restoreBoard(
			ctx,
			prepared.reader,
			preparedBoard,
		)
		if err != nil {
			return RestoreResult{}, fmt.Errorf(
				"restore board %q: %w",
				preparedBoard.publication.SourceBoardID,
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
	reader *Reader,
	prepared preparedBoard,
) (boardcopy.CopyOutcome, error) {
	index := prepared.index
	options := boardcopy.CopyOptions{
		ProjectID: prepared.publication.ProjectID.String(),
	}
	receipt, found, err := s.boards.ReadCopyReceipt(
		ctx,
		boardcopy.CopyReceiptKey{
			SourceLineageID: index.Header.SourceLineageID,
			SourceBoardID:   index.Header.Board.ID,
			SnapshotVersion: index.Header.Version,
		},
	)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	if found {
		outcome, err := boardcopy.EvaluateRecordReceipt(index, options, receipt)
		if err != nil {
			return boardcopy.CopyOutcome{}, err
		}
		outcome.Counts.Blobs = len(index.Blobs)
		return outcome, nil
	}

	records, err := reader.OpenBoard(prepared.publication)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	imported, err := s.boards.ImportCopyRecords(ctx, index, records, options)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	outcome, err := boardcopy.EvaluateCopyImport(index, options, imported)
	if err != nil {
		return boardcopy.CopyOutcome{}, err
	}
	outcome.Counts.Blobs = len(index.Blobs)
	return outcome, nil
}
