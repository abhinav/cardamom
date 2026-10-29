package board

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"iter"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// CloneRepository publishes replicas between two distinct, open stores.
// Its caller owns those stores and the attachment repositories' lifetimes.
// Each call holds one source snapshot and one destination writer transaction
// until shared projection, history, and blob references can commit together.
type CloneRepository struct {
	// Source provides the retained shared state and history.
	Source *store.Store // required

	// Destination receives the new board in one transaction.
	Destination *store.Store // required

	// SourceBlobs opens verified CAS bytes retained by the source history.
	SourceBlobs CloneBlobSource // required

	// DestinationBlobs publishes verified CAS bytes before metadata commits.
	DestinationBlobs CloneBlobDestination // required

	// Entropy supplies the new replica's writer identity.
	Entropy io.Reader // required

	// SourceConfiguration is the source store's local configuration layer.
	SourceConfiguration configuration.Overrides

	// DestinationConfiguration is the destination store's local configuration.
	DestinationConfiguration configuration.Overrides
}

// CloneBlobSource opens immutable content addressed by a verified descriptor.
type CloneBlobSource interface {
	OpenCopyBlob(context.Context, attachment.BlobDescriptor) (io.ReadCloser, error)
}

// CloneBlobDestination publishes immutable content idempotently.
type CloneBlobDestination interface {
	PublishCopyBlob(context.Context, attachment.BlobDescriptor, io.Reader) error
}

var _ boardclone.Repository = (*CloneRepository)(nil)

// Clone preserves a board's shared identity and history while assigning a new
// local writer. Claims, assignees, upload sessions, mail, and leases remain in
// their original store. Failure before commit leaves no destination board;
// verified CAS bytes may remain for ordinary orphan collection. A cleanup
// failure after commit returns the committed result alongside the error.
func (r *CloneRepository) Clone(ctx context.Context, request boardclone.Request) (out boardclone.Result, err error) {
	if r.Source == r.Destination {
		return out, errors.New("source and destination stores must differ")
	}
	view, err := r.Source.View(ctx)
	if err != nil {
		return out, fmt.Errorf("begin source clone snapshot: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	reader := query.New(view)
	header, err := reader.ProjectGetCloneBoard(ctx, request.BoardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "source board %q not found", request.BoardID)
	}
	if err != nil {
		return out, fmt.Errorf("read source board: %w", err)
	}
	change, err := r.Destination.Change(ctx)
	if err != nil {
		return out, fmt.Errorf("begin destination clone: %w", err)
	}
	defer func() { err = errors.Join(err, change.Done()) }()
	writer := query.New(change)
	if err := r.checkDestination(ctx, reader, writer, request, header); err != nil {
		return out, err
	}
	importer, err := cloneProjectionImporter(ctx, reader, writer, request.BoardID)
	if err != nil {
		return out, err
	}
	writerUID, err := r.newWriter(ctx, reader, request.BoardID)
	if err != nil {
		return out, err
	}

	// Two local revisions let removed attachment metadata retain its strict
	// created-before-removed ordering. Revision numbers are not shared history.
	revisions, err := change.ReserveRevisions(ctx, 2)
	if err != nil {
		return out, fmt.Errorf("reserve clone revisions: %w", err)
	}
	if err := writer.ProjectInsertClonedBoard(ctx, query.ProjectInsertClonedBoardParams{
		ID: header.ID, ProjectID: request.ProjectID.String(), Name: header.Name,
		Description: header.Description, CreatedAt: header.CreatedAt,
		ArchivedAt: header.ArchivedAt, ArchivedBy: header.ArchivedBy,
		ArchiveReason: header.ArchiveReason,
		IssueIDPrefix: header.IssueIDPrefix, IssueIDStrategy: header.IssueIDStrategy,
		IssueSummaryMaxBytes: header.IssueSummaryMaxBytes,
		AttachmentMaxBytes:   header.AttachmentMaxBytes,
		BoardPinsMaxCount:    header.BoardPinsMaxCount, Revision: revisions.LastRevision(),
	}); err != nil {
		return out, fmt.Errorf("create cloned board: %w", err)
	}
	if err := writer.ProjectInsertBoardReplicaIdentity(ctx, query.ProjectInsertBoardReplicaIdentityParams{
		BoardID: header.ID, WriterUid: writerUID.Bytes(),
	}); err != nil {
		return out, fmt.Errorf("create cloned writer identity: %w", err)
	}

	// Destination collection also takes a writer transaction. Hold ours while
	// publishing bytes so collection cannot remove them before their references
	// commit. Source history retains every historical blob independently of the
	// current attachment lifecycle, so the source snapshot needs no blob lease.
	blobCount, err := r.cloneBlobs(ctx, reader, writer, request.BoardID)
	if err != nil {
		return out, err
	}
	importer.firstRevision, importer.lastRevision = revisions.FirstRevision(), revisions.LastRevision()
	pager := copyRecordPager{ctx: ctx, queries: reader, boardID: header.ID, pageSize: copyRecordPageSize}
	for record, err := range cloneProjectionRecords(&pager) {
		if err != nil {
			return out, err
		}
		if err := importer.Import(record); err != nil {
			return out, err
		}
	}
	mutationCount, err := repositoryhistory.CloneV1(ctx, view, change, request.BoardID)
	if err != nil {
		return out, err
	}
	if err := change.PublishRevisions(ctx, revisions); err != nil {
		return out, fmt.Errorf("publish clone revisions: %w", err)
	}
	if err := change.Commit(); err != nil {
		return out, fmt.Errorf("commit cloned board: %w", err)
	}
	return boardclone.Result{
		BoardID: header.ID, Name: header.Name, ProjectID: request.ProjectID.String(),
		Issues: len(importer.issueUIDs), Mutations: mutationCount, Blobs: blobCount,
	}, nil
}

func (r *CloneRepository) checkDestination(
	ctx context.Context,
	reader, writer *query.Queries,
	request boardclone.Request,
	header query.ProjectGetCloneBoardRow,
) error {
	exists, err := writer.ProjectCopyBoardIDExists(ctx, header.ID)
	if err != nil {
		return fmt.Errorf("inspect destination board identity: %w", err)
	}
	if exists {
		return errkind.Errorf(errkind.Conflict, "destination already contains board %q; clone does not update an existing replica", header.ID)
	}
	if err := requireCopyProject(ctx, writer, request.ProjectID.String()); err != nil {
		return err
	}
	exists, err = writer.ProjectCopyBoardNameExists(ctx, query.ProjectCopyBoardNameExistsParams{
		ProjectID: request.ProjectID.String(), Name: header.Name,
	})
	if err != nil {
		return fmt.Errorf("inspect destination board name: %w", err)
	}
	if exists {
		return errkind.Errorf(errkind.Conflict, "destination project already contains a board named %q", header.Name)
	}
	// Strategy checks resolve local defaults only to establish eligibility.
	// Cloning transfers nullable board overrides, never these resolved values.
	layers, err := reader.ProjectGetConfigurationLayers(ctx, header.ID)
	if err != nil {
		return fmt.Errorf("read source configuration: %w", err)
	}
	sourceProject, err := copyOverrides(layers.ProjectIssueIDPrefix, layers.ProjectIssueIDStrategy,
		layers.ProjectIssueSummaryMaxBytes, layers.ProjectAttachmentMaxBytes, layers.ProjectBoardPinsMaxCount)
	if err != nil {
		return err
	}
	boardOverrides, err := copyOverrides(header.IssueIDPrefix, header.IssueIDStrategy,
		header.IssueSummaryMaxBytes, header.AttachmentMaxBytes, header.BoardPinsMaxCount)
	if err != nil {
		return err
	}
	if err := boardclone.RequireRandomIDs("source",
		resolveCopyConfiguration(r.SourceConfiguration, sourceProject, boardOverrides).Issue.ID.Strategy); err != nil {
		return err
	}
	projectRow, err := writer.ProjectGetProjectConfiguration(ctx, request.ProjectID.String())
	if err != nil {
		return fmt.Errorf("read destination configuration: %w", err)
	}
	destinationProject, err := copyOverrides(projectRow.IssueIDPrefix, projectRow.IssueIDStrategy,
		projectRow.IssueSummaryMaxBytes, projectRow.AttachmentMaxBytes, projectRow.BoardPinsMaxCount)
	if err != nil {
		return err
	}
	return boardclone.RequireRandomIDs("destination",
		resolveCopyConfiguration(r.DestinationConfiguration, destinationProject, boardOverrides).Issue.ID.Strategy)
}

func (r *CloneRepository) newWriter(ctx context.Context, reader *query.Queries, boardID board.ID) (board.WriterUID, error) {
	current, err := reader.BoardHistoryGetWriterUID(ctx, boardID.String())
	if err != nil {
		return board.WriterUID{}, fmt.Errorf("read source writer: %w", err)
	}
	tips, err := reader.BoardHistoryListTips(ctx, boardID.String())
	if err != nil {
		return board.WriterUID{}, fmt.Errorf("read source writer tips: %w", err)
	}
	writer, err := board.GenerateWriterUID(r.Entropy)
	if err != nil {
		return board.WriterUID{}, err
	}
	if bytes.Equal(writer.Bytes(), current) {
		return board.WriterUID{}, errors.New("new writer identity collides with source writer")
	}
	for _, tip := range tips {
		if bytes.Equal(writer.Bytes(), tip.WriterUid) {
			return board.WriterUID{}, errors.New("new writer identity collides with retained history")
		}
	}
	return writer, nil
}

func (r *CloneRepository) cloneBlobs(ctx context.Context, reader, writer *query.Queries, boardID board.ID) (int, error) {
	var afterDigest string
	count := 0
	for {
		row, err := reader.AttachmentCloneNextBlob(ctx, query.AttachmentCloneNextBlobParams{
			BoardID: boardID.String(), AfterDigest: afterDigest,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return count, nil
		}
		if err != nil {
			return 0, fmt.Errorf("read cloned blob descriptor: %w", err)
		}
		digest, err := attachment.NewDigest(row.Digest)
		if err != nil {
			return 0, err
		}
		if row.SizeBytes < 0 {
			return 0, errors.New("source blob has negative size")
		}
		descriptor := attachment.BlobDescriptor{Digest: digest, SizeBytes: uint64(row.SizeBytes)}
		content, err := r.SourceBlobs.OpenCopyBlob(ctx, descriptor)
		if err != nil {
			return 0, fmt.Errorf("open source blob %q: %w", row.Digest, err)
		}
		err = r.DestinationBlobs.PublishCopyBlob(ctx, descriptor, content)
		err = errors.Join(err, content.Close())
		if err != nil {
			return 0, fmt.Errorf("publish cloned blob %q: %w", row.Digest, err)
		}
		if err := writer.AttachmentRetainBlob(ctx, query.AttachmentRetainBlobParams(row)); err != nil {
			return 0, fmt.Errorf("retain cloned blob descriptor: %w", err)
		}
		afterDigest = row.Digest
		count++
	}
}

// cloneProjectionImporter reuses the row writer used by copy and restore, not
// their duplication policy. Every identity maps to itself and Markdown passes
// through untouched. Board metadata and history have separate clone writers;
// no copy header, receipt, or newly generated issue UID enters this importer.
func cloneProjectionImporter(ctx context.Context, reader, writer *query.Queries, boardID board.ID) (*copyRecordImporter, error) {
	issues, err := reader.BoardCloneIssueIdentities(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read clone issue identities: %w", err)
	}
	importer := &copyRecordImporter{
		ctx: ctx, queries: writer, boardID: boardID.String(),
		issueIDs: make(map[string]string, len(issues)), issueUIDs: make(map[string]issue.UID, len(issues)),
		logIDs: make(map[string]string), rewrite: func(value string) string { return value },
	}
	for _, row := range issues {
		uid, err := issue.ParseUID(row.Uid)
		if err != nil {
			return nil, err
		}
		publicExists, err := writer.BoardIssueIDExists(ctx, row.ID)
		if err != nil {
			return nil, fmt.Errorf("inspect destination issue ID: %w", err)
		}
		privateExists, err := writer.BoardIssueUIDExists(ctx, row.Uid)
		if err != nil {
			return nil, fmt.Errorf("inspect destination issue UID: %w", err)
		}
		if publicExists || privateExists {
			return nil, errkind.Errorf(errkind.Conflict, "destination issue identity collides with source issue %q; clone preserves identities", row.ID)
		}
		importer.issueIDs[row.ID], importer.issueUIDs[row.ID] = row.ID, uid
	}
	logs, err := reader.BoardCloneLogIdentities(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read clone Log identities: %w", err)
	}
	for _, id := range logs {
		exists, err := writer.BoardCopyLogIDExists(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("inspect destination Log identity: %w", err)
		}
		if exists {
			return nil, errkind.Errorf(errkind.Conflict, "destination Log identity %q collides with source", id)
		}
		importer.logIDs[id] = id
	}
	return importer, nil
}

func cloneProjectionRecords(pager *copyRecordPager) iter.Seq2[boardcopy.Record, error] {
	return func(yield func(boardcopy.Record, error) bool) { pager.read(yield) }
}
