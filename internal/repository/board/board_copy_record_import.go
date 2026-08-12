package board

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// copyRecordImporter writes one validated publication into an unpublished
// destination board. Its caller owns validation and transaction publication.
type copyRecordImporter struct {
	ctx           context.Context // required
	queries       *query.Queries  // required
	projectID     string          // required
	name          string          // required
	boardID       string          // required
	issueIDs      map[string]string
	logIDs        map[string]string
	rewrite       func(string) string // required
	firstRevision int64
	lastRevision  int64
}

// Import persists one record using the complete source identity mappings.
func (i *copyRecordImporter) Import(record boardcopy.Record) error {
	switch value := record.(type) {
	case boardcopy.RecordHeader:
		return i.importHeader(value)
	case boardcopy.CopyIssue:
		return i.importIssue(value)
	case boardcopy.CopyLabel:
		return i.importLabel(value)
	case boardcopy.CopyDependency:
		return i.importDependency(value)
	case boardcopy.CopyContainment:
		return i.importContainment(value)
	case boardcopy.CopyExternalKey:
		return i.importExternalKey(value)
	case boardcopy.CopyLogEntry:
		return i.importLogEntry(value)
	case boardcopy.CopyState:
		return i.importState(value)
	case boardcopy.CopyResultRecord:
		return i.importResult(value)
	case boardcopy.CopyCheckpoint:
		return i.importCheckpoint(value)
	case boardcopy.CopyAttachment:
		return i.importAttachment(value)
	case boardcopy.CopyPin:
		return i.importPin(value)
	case boardcopy.RecordTrailer:
		return nil
	default:
		return fmt.Errorf("unsupported destination board record %T", record)
	}
}

func (i *copyRecordImporter) importHeader(value boardcopy.RecordHeader) error {
	description := rewriteOptionalCopyMarkdown(value.Board.Description, i.rewrite)
	prefix := value.Configuration.Issue.ID.Prefix.String()
	strategy := value.Configuration.Issue.ID.Strategy.String()
	summaryMaxBytes := int64(value.Configuration.Issue.Summary.MaxBytes.Uint64())
	attachmentMaxBytes := int64(value.Configuration.Attachment.MaxBytes.Uint64())
	pinMaxCount := int64(value.Configuration.Board.Pins.MaxCount.Uint64())
	if err := i.queries.ProjectInsertCopiedBoard(
		i.ctx,
		query.ProjectInsertCopiedBoardParams{
			ID: i.boardID, ProjectID: i.projectID, Name: i.name,
			Description: description, CreatedAt: value.Board.CreatedAt,
			IssueIDPrefix: &prefix, IssueIDStrategy: &strategy,
			IssueSummaryMaxBytes: &summaryMaxBytes,
			AttachmentMaxBytes:   &attachmentMaxBytes,
			BoardPinsMaxCount:    &pinMaxCount,
			Revision:             i.lastRevision,
		},
	); err != nil {
		return fmt.Errorf("create destination board: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importIssue(value boardcopy.CopyIssue) error {
	issueID, err := i.issueID(value.ID)
	if err != nil {
		return err
	}
	err = i.queries.BoardInsertCopiedIssue(
		i.ctx,
		query.BoardInsertCopiedIssueParams{
			ID: issueID, BoardID: i.boardID, Title: value.Title,
			Kind: value.Kind, Lifecycle: value.Lifecycle,
			Priority: value.Priority, CreatedAt: value.CreatedAt,
			UpdatedAt: value.UpdatedAt, ClosedAt: value.ClosedAt,
			WaitingReason: value.WaitingReason, WaitingSince: value.WaitingSince,
			Summary:  rewriteOptionalCopyMarkdown(value.Summary, i.rewrite),
			Details:  rewriteOptionalCopyMarkdown(value.Details, i.rewrite),
			Revision: i.lastRevision,
		},
	)
	if err != nil {
		return fmt.Errorf("create destination issue %q: %w", value.ID, err)
	}
	return nil
}

func (i *copyRecordImporter) importLabel(value boardcopy.CopyLabel) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertIssueLabel(
		i.ctx,
		query.BoardInsertIssueLabelParams{
			BoardID: i.boardID, IssueID: issueID, Label: value.Label,
		},
	); err != nil {
		return fmt.Errorf("create destination issue label: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importDependency(
	value boardcopy.CopyDependency,
) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	prerequisiteID, err := i.issueID(value.PrerequisiteID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertIssueDependency(
		i.ctx,
		query.BoardInsertIssueDependencyParams{
			BoardID: i.boardID, IssueID: issueID,
			PrerequisiteID: prerequisiteID,
		},
	); err != nil {
		return fmt.Errorf("create destination dependency: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importContainment(
	value boardcopy.CopyContainment,
) error {
	childID, err := i.issueID(value.ChildID)
	if err != nil {
		return err
	}
	parentID, err := i.issueID(value.ParentID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertIssueParent(
		i.ctx,
		query.BoardInsertIssueParentParams{
			BoardID: i.boardID, ChildID: childID, ParentID: parentID,
		},
	); err != nil {
		return fmt.Errorf("create destination containment: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importExternalKey(
	value boardcopy.CopyExternalKey,
) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertIssueExternalKey(
		i.ctx,
		query.BoardInsertIssueExternalKeyParams{
			BoardID: i.boardID, ExternalKey: value.Key, IssueID: issueID,
		},
	); err != nil {
		return fmt.Errorf("create destination external key: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importLogEntry(
	value boardcopy.CopyLogEntry,
) error {
	logID, err := i.logID(value.ID)
	if err != nil {
		return err
	}
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertIssueLogEntry(
		i.ctx,
		query.BoardInsertIssueLogEntryParams{
			ID: logID, BoardID: i.boardID, IssueID: issueID,
			Kind: value.Kind, Author: value.Author, Committer: value.Committer,
			Body: i.rewrite(value.Body), CreatedAt: value.CreatedAt,
			NextAction: rewriteOptionalCopyMarkdown(value.NextAction, i.rewrite),
		},
	); err != nil {
		return fmt.Errorf("create destination Log entry %q: %w", value.ID, err)
	}
	return nil
}

func (i *copyRecordImporter) importState(value boardcopy.CopyState) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	var snapshotID *string
	if value.SnapshotLogEntryID != nil {
		mapped, err := i.logID(*value.SnapshotLogEntryID)
		if err != nil {
			return err
		}
		snapshotID = &mapped
	}
	if err := i.queries.BoardUpsertIssueState(
		i.ctx,
		query.BoardUpsertIssueStateParams{
			IssueID: issueID, BoardID: i.boardID, Body: i.rewrite(value.Body),
			Author: value.Author, UpdatedAt: value.UpdatedAt,
			SnapshotLogEntryID: snapshotID,
			NextAction:         rewriteOptionalCopyMarkdown(value.NextAction, i.rewrite),
		},
	); err != nil {
		return fmt.Errorf("create destination State: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importResult(
	value boardcopy.CopyResultRecord,
) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardUpsertIssueResult(
		i.ctx,
		query.BoardUpsertIssueResultParams{
			IssueID: issueID, BoardID: i.boardID, Body: i.rewrite(value.Body),
		},
	); err != nil {
		return fmt.Errorf("create destination Result: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importCheckpoint(
	value boardcopy.CopyCheckpoint,
) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertCheckpointDecision(
		i.ctx,
		query.BoardInsertCheckpointDecisionParams{
			IssueID: issueID, BoardID: i.boardID, Outcome: value.Outcome,
			Reason: i.rewrite(value.Reason), DecidedAt: value.DecidedAt,
			Revision: i.lastRevision,
		},
	); err != nil {
		return fmt.Errorf("create destination checkpoint decision: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) importAttachment(
	value boardcopy.CopyAttachment,
) error {
	if err := i.queries.AttachmentRetainBlob(
		i.ctx,
		query.AttachmentRetainBlobParams{
			Digest:    value.Blob.Digest.String(),
			SizeBytes: int64(value.Blob.SizeBytes),
		},
	); err != nil {
		return fmt.Errorf("create destination blob descriptor: %w", err)
	}
	var originIssueID *string
	if value.OriginIssueID != nil {
		mapped, err := i.issueID(*value.OriginIssueID)
		if err != nil {
			return err
		}
		originIssueID = &mapped
	}
	createdRevision := i.lastRevision
	var removedRevision *int64
	if value.Lifecycle == attachment.LifecycleRemoved.String() {
		createdRevision = i.firstRevision
		removedRevision = &i.lastRevision
	}
	if err := i.queries.AttachmentInsertCopiedMetadata(
		i.ctx,
		query.AttachmentInsertCopiedMetadataParams{
			BoardID: i.boardID, ID: value.ID, OriginIssueID: originIssueID,
			BlobDigest:    value.Blob.Digest.String(),
			BlobSizeBytes: int64(value.Blob.SizeBytes),
			Filename:      value.Filename, MediaType: value.MediaType,
			Lifecycle: value.Lifecycle, CreatedActor: value.CreatedActor,
			CreatedAt: value.CreatedAt, CreatedRevision: createdRevision,
			RemovedActor: value.RemovedActor, RemovedAt: value.RemovedAt,
			RemovedRevision: removedRevision,
		},
	); err != nil {
		return fmt.Errorf("create destination attachment %q: %w", value.ID, err)
	}
	return nil
}

func (i *copyRecordImporter) importPin(value boardcopy.CopyPin) error {
	issueID, err := i.issueID(value.IssueID)
	if err != nil {
		return err
	}
	if err := i.queries.BoardInsertPin(
		i.ctx,
		query.BoardInsertPinParams{BoardID: i.boardID, IssueID: issueID},
	); err != nil {
		return fmt.Errorf("create destination board pin: %w", err)
	}
	return nil
}

func (i *copyRecordImporter) issueID(source string) (string, error) {
	destination, found := i.issueIDs[source]
	if !found {
		return "", fmt.Errorf("source issue %q is absent from record index", source)
	}
	return destination, nil
}

func (i *copyRecordImporter) logID(source string) (string, error) {
	destination, found := i.logIDs[source]
	if !found {
		return "", fmt.Errorf("source Log entry %q is absent from record index", source)
	}
	return destination, nil
}
