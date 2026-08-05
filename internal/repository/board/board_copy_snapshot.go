package board

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/attachment"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// ReadCopySnapshot reads one complete semantic board snapshot from a retained
// SQLite view.
func (r *Repository) ReadCopySnapshot(
	ctx context.Context,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
) (snapshot boardcopy.CopySnapshot, err error) {
	if err := storeOverrides.Validate(); err != nil {
		return snapshot, fmt.Errorf("source store configuration: %w", err)
	}
	if boardID != r.boardID {
		return snapshot, errkind.Errorf(
			errkind.InvalidInput,
			"board copy source %q does not match repository board %q",
			boardID,
			r.boardID,
		)
	}
	view, err := r.store.View(ctx)
	if err != nil {
		return snapshot, fmt.Errorf("begin board copy snapshot: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	lineageID, err := view.LineageID(ctx)
	if err != nil {
		return snapshot, err
	}
	revision, err := view.CanonicalRevision(ctx)
	if err != nil {
		return snapshot, fmt.Errorf("read source revision: %w", err)
	}
	return readCopySnapshot(
		ctx,
		view,
		boardID,
		storeOverrides,
		BackupSource{LineageID: lineageID, Revision: revision},
		requireQuiescentSnapshot,
	)
}

// BackupSource identifies the retained source store view shared by every board
// in one backup capture.
type BackupSource struct {
	// LineageID identifies the source store persistence history.
	LineageID string // required

	// Revision is the canonical source revision retained by the view.
	Revision int64
}

// BackupReader reads complete semantic board snapshots from a caller-owned
// retained repository view.
type BackupReader struct{}

// copySnapshotPolicy controls whether ephemeral activity gates semantic
// snapshot capture.
type copySnapshotPolicy uint8

const (
	// captureCommittedSnapshot omits ephemeral records without inspecting them.
	captureCommittedSnapshot copySnapshotPolicy = iota

	// requireQuiescentSnapshot preserves board-copy's operational quarantine.
	requireQuiescentSnapshot
)

// ReadBackupSnapshot reads committed semantic board state while omitting
// ephemeral claims and attachment uploads.
func (r *BackupReader) ReadBackupSnapshot(
	ctx context.Context,
	view *store.View,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	source BackupSource,
) (boardcopy.CopySnapshot, error) {
	return readCopySnapshot(
		ctx,
		view,
		boardID,
		storeOverrides,
		source,
		captureCommittedSnapshot,
	)
}

func readCopySnapshot(
	ctx context.Context,
	view *store.View,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	sourceView BackupSource,
	policy copySnapshotPolicy,
) (snapshot boardcopy.CopySnapshot, err error) {
	if err := storeOverrides.Validate(); err != nil {
		return snapshot, fmt.Errorf("source store configuration: %w", err)
	}
	if view == nil {
		return snapshot, errors.New("source store view is required")
	}
	if strings.TrimSpace(sourceView.LineageID) == "" {
		return snapshot, errors.New("source store lineage is required")
	}
	if sourceView.Revision < 0 {
		return snapshot, errors.New("source store revision cannot be negative")
	}
	snapshot.SourceLineageID = sourceView.LineageID
	snapshot.SourceRevision = sourceView.Revision

	queries := query.New(view)
	source, err := queries.BoardGetCopySource(ctx, boardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return snapshot, fmt.Errorf("read source board: %w", err)
	}
	snapshot.Board = boardcopy.CopyBoard{
		ID:          source.BoardID,
		Name:        source.BoardName,
		Description: source.BoardDescription,
		CreatedAt:   source.BoardCreatedAt,
	}
	projectLayer, err := copyOverrides(
		source.ProjectIssueIDPrefix,
		source.ProjectIssueIDStrategy,
		source.ProjectIssueSummaryMaxBytes,
		source.ProjectAttachmentMaxBytes,
	)
	if err != nil {
		return snapshot, fmt.Errorf("load source project configuration: %w", err)
	}
	boardLayer, err := copyOverrides(
		source.BoardIssueIDPrefix,
		source.BoardIssueIDStrategy,
		source.BoardIssueSummaryMaxBytes,
		source.BoardAttachmentMaxBytes,
	)
	if err != nil {
		return snapshot, fmt.Errorf("load source board configuration: %w", err)
	}
	snapshot.Configuration = resolveCopyConfiguration(
		storeOverrides,
		projectLayer,
		boardLayer,
	)

	if policy == requireQuiescentSnapshot {
		activeClaims, err := queries.BoardCountCopyActiveClaims(
			ctx,
			boardID.String(),
		)
		if err != nil {
			return snapshot, fmt.Errorf("inspect source claims: %w", err)
		}
		if activeClaims != 0 {
			return snapshot, errkind.Errorf(
				errkind.Conflict,
				"source board has active claims",
			)
		}
		activeUploads, err := queries.AttachmentCountCopyActiveUploads(
			ctx,
			boardID.String(),
		)
		if err != nil {
			return snapshot, fmt.Errorf("inspect source attachment uploads: %w", err)
		}
		if activeUploads != 0 {
			return snapshot, errkind.Errorf(
				errkind.Conflict,
				"source board has active attachment uploads",
			)
		}
	}

	if snapshot.Issues, err = readCopyIssues(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Labels, err = readCopyLabels(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Dependencies, err = readCopyDependencies(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Containment, err = readCopyContainment(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.ExternalKeys, err = readCopyExternalKeys(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.LogEntries, err = readCopyLogEntries(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.States, err = readCopyStates(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Results, err = readCopyResults(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Checkpoints, err = readCopyCheckpoints(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	if snapshot.Attachments, err = readCopyAttachments(ctx, queries, boardID); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

func copyOverrides(
	prefixValue *string,
	strategyValue *string,
	summaryMax *int64,
	attachmentMax *int64,
) (configuration.Overrides, error) {
	var overrides configuration.Overrides
	if prefixValue != nil {
		value, err := configuration.NewPrefix(*prefixValue)
		if err != nil {
			return overrides, err
		}
		overrides.Issue.ID.Prefix = &value
	}
	if strategyValue != nil {
		value, err := configuration.NewIDStrategy(*strategyValue)
		if err != nil {
			return overrides, err
		}
		overrides.Issue.ID.Strategy = &value
	}
	if summaryMax != nil {
		value, err := configuration.NewByteLimit(uint64(*summaryMax))
		if err != nil {
			return overrides, err
		}
		overrides.Issue.Summary.MaxBytes = &value
	}
	if attachmentMax != nil {
		value, err := configuration.NewByteLimit(uint64(*attachmentMax))
		if err != nil {
			return overrides, err
		}
		overrides.Attachment.MaxBytes = &value
	}
	return overrides, nil
}

func resolveCopyConfiguration(
	layers ...configuration.Overrides,
) configuration.Configuration {
	resolved := configuration.Defaults()
	for _, layer := range layers {
		if value := layer.Issue.ID.Prefix; value != nil {
			resolved.Issue.ID.Prefix = *value
		}
		if value := layer.Issue.ID.Strategy; value != nil {
			resolved.Issue.ID.Strategy = *value
		}
		if value := layer.Issue.Summary.MaxBytes; value != nil {
			resolved.Issue.Summary.MaxBytes = *value
		}
		if value := layer.Attachment.MaxBytes; value != nil {
			resolved.Attachment.MaxBytes = *value
		}
	}
	return resolved
}

func readCopyIssues(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyIssue, error) {
	rows, err := queries.BoardListCopyIssues(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source issues: %w", err)
	}
	out := make([]boardcopy.CopyIssue, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyIssue{
			ID:            row.ID,
			Title:         row.Title,
			Kind:          row.Kind,
			Lifecycle:     row.Lifecycle,
			Priority:      row.Priority,
			CreatedAt:     row.CreatedAt,
			UpdatedAt:     row.UpdatedAt,
			ClosedAt:      row.ClosedAt,
			WaitingReason: row.WaitingReason,
			WaitingSince:  row.WaitingSince,
			Summary:       row.Summary,
			Details:       row.Details,
		})
	}
	return out, nil
}

func readCopyLabels(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyLabel, error) {
	rows, err := queries.BoardListCopyLabels(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source issue labels: %w", err)
	}
	out := make([]boardcopy.CopyLabel, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyLabel{
			IssueID: row.IssueID,
			Label:   row.Label,
		})
	}
	return out, nil
}

func readCopyDependencies(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyDependency, error) {
	rows, err := queries.BoardListCopyDependencies(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source dependencies: %w", err)
	}
	out := make([]boardcopy.CopyDependency, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyDependency{
			IssueID:        row.IssueID,
			PrerequisiteID: row.PrerequisiteID,
		})
	}
	return out, nil
}

func readCopyContainment(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyContainment, error) {
	rows, err := queries.BoardListCopyContainment(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source containment: %w", err)
	}
	out := make([]boardcopy.CopyContainment, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyContainment{
			ChildID:  row.ChildID,
			ParentID: row.ParentID,
		})
	}
	return out, nil
}

func readCopyExternalKeys(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyExternalKey, error) {
	rows, err := queries.BoardListCopyExternalKeys(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source external keys: %w", err)
	}
	out := make([]boardcopy.CopyExternalKey, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyExternalKey{
			Key:     row.ExternalKey,
			IssueID: row.IssueID,
		})
	}
	return out, nil
}

func readCopyLogEntries(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyLogEntry, error) {
	rows, err := queries.BoardListCopyLogEntries(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source Log entries: %w", err)
	}
	out := make([]boardcopy.CopyLogEntry, 0, len(rows))
	for order, row := range rows {
		out = append(out, boardcopy.CopyLogEntry{
			Order:      uint64(order),
			ID:         row.ID,
			IssueID:    row.IssueID,
			Kind:       row.Kind,
			Author:     row.Author,
			Committer:  row.Committer,
			Body:       row.Body,
			CreatedAt:  row.CreatedAt,
			NextAction: row.NextAction,
		})
	}
	return out, nil
}

func readCopyStates(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyState, error) {
	rows, err := queries.BoardListCopyStates(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source States: %w", err)
	}
	out := make([]boardcopy.CopyState, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyState{
			IssueID:            row.IssueID,
			Body:               row.Body,
			Author:             row.Author,
			UpdatedAt:          row.UpdatedAt,
			SnapshotLogEntryID: row.SnapshotLogEntryID,
			NextAction:         row.NextAction,
		})
	}
	return out, nil
}

func readCopyResults(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyResultRecord, error) {
	rows, err := queries.BoardListCopyResults(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source Results: %w", err)
	}
	out := make([]boardcopy.CopyResultRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyResultRecord{
			IssueID: row.IssueID,
			Body:    row.Body,
		})
	}
	return out, nil
}

func readCopyCheckpoints(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyCheckpoint, error) {
	rows, err := queries.BoardListCopyCheckpoints(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source checkpoint decisions: %w", err)
	}
	out := make([]boardcopy.CopyCheckpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, boardcopy.CopyCheckpoint{
			IssueID:   row.IssueID,
			Outcome:   row.Outcome,
			Reason:    row.Reason,
			DecidedAt: row.DecidedAt,
		})
	}
	return out, nil
}

func readCopyAttachments(
	ctx context.Context,
	queries *query.Queries,
	boardID domainboard.ID,
) ([]boardcopy.CopyAttachment, error) {
	rows, err := queries.AttachmentListCopyMetadata(ctx, boardID.String())
	if err != nil {
		return nil, fmt.Errorf("read source attachments: %w", err)
	}
	out := make([]boardcopy.CopyAttachment, 0, len(rows))
	for _, row := range rows {
		digest, err := attachment.NewDigest(row.BlobDigest)
		if err != nil {
			return nil, fmt.Errorf("load source attachment digest: %w", err)
		}
		blob := attachment.BlobDescriptor{
			Digest: digest, SizeBytes: uint64(row.BlobSizeBytes),
		}
		if err := blob.Validate(); err != nil {
			return nil, fmt.Errorf("load source attachment blob: %w", err)
		}
		out = append(out, boardcopy.CopyAttachment{
			ID:            row.ID,
			OriginIssueID: row.OriginIssueID,
			Blob:          blob,
			Filename:      row.Filename,
			MediaType:     row.MediaType,
			Lifecycle:     row.Lifecycle,
			CreatedActor:  row.CreatedActor,
			CreatedAt:     row.CreatedAt,
			RemovedActor:  row.RemovedActor,
			RemovedAt:     row.RemovedAt,
		})
	}
	return out, nil
}
