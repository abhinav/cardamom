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

const copyRecordPageSize = 128

// BackupSource identifies the retained source store view shared by every board
// in one backup capture.
type BackupSource struct {
	// LineageID identifies the source store persistence history.
	LineageID string // required

	// Revision is the canonical source revision retained by the view.
	Revision int64
}

// BackupReader reads semantic board records from a caller-owned retained view.
type BackupReader struct{}

type copyRecordPolicy uint8

const (
	captureCommittedRecords copyRecordPolicy = iota
	requireQuiescentRecords
)

var _ boardcopy.RecordSource = (*Repository)(nil)

// ReadCopyRecords yields one semantic board publication from an owned retained
// view. The view is released when iteration completes or the consumer stops.
func (r *Repository) ReadCopyRecords(
	ctx context.Context,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
) boardcopy.RecordSequence {
	return r.readCopyRecords(
		ctx,
		boardID,
		storeOverrides,
		copyRecordPageSize,
	)
}

func (r *Repository) readCopyRecords(
	ctx context.Context,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	pageSize int64,
) boardcopy.RecordSequence {
	return func(yield func(boardcopy.Record, error) bool) {
		view, err := r.store.View(ctx)
		if err != nil {
			yield(nil, fmt.Errorf("begin board copy records: %w", err))
			return
		}
		mayYield := true
		defer func() {
			err := view.Done()
			if err != nil && mayYield {
				yield(nil, fmt.Errorf("close board copy records: %w", err))
			}
		}()

		if boardID != r.boardID {
			yield(nil, errkind.Errorf(
				errkind.InvalidInput,
				"board copy source %q does not match repository board %q",
				boardID,
				r.boardID,
			))
			return
		}
		lineageID, err := view.LineageID(ctx)
		if err != nil {
			yield(nil, err)
			return
		}
		revision, err := view.CanonicalRevision(ctx)
		if err != nil {
			yield(nil, fmt.Errorf("read source revision: %w", err))
			return
		}
		sequence := readCopyRecordSequence(
			ctx,
			view,
			boardID,
			storeOverrides,
			BackupSource{LineageID: lineageID, Revision: revision},
			requireQuiescentRecords,
			pageSize,
		)
		for record, err := range sequence {
			if !yield(record, err) {
				mayYield = false
				return
			}
			if err != nil {
				return
			}
		}
	}
}

// ReadBackupRecords yields one semantic board publication from a retained view
// owned by the caller. Iteration never closes view.
func (r *BackupReader) ReadBackupRecords(
	ctx context.Context,
	view *store.View,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	source BackupSource,
) boardcopy.RecordSequence {
	return readCopyRecordSequence(
		ctx,
		view,
		boardID,
		storeOverrides,
		source,
		captureCommittedRecords,
		copyRecordPageSize,
	)
}

func readCopyRecordSequence(
	ctx context.Context,
	view *store.View,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	source BackupSource,
	policy copyRecordPolicy,
	pageSize int64,
) boardcopy.RecordSequence {
	return func(yield func(boardcopy.Record, error) bool) {
		header, err := readCopyRecordHeader(
			ctx,
			view,
			boardID,
			storeOverrides,
			source,
			policy,
		)
		if err != nil {
			yield(nil, err)
			return
		}
		if !yield(header, nil) {
			return
		}

		pager := copyRecordPager{
			ctx: ctx, queries: query.New(view), boardID: boardID.String(),
			pageSize: pageSize,
		}
		if !pager.read(yield) {
			return
		}
		yield(boardcopy.RecordTrailer{Counts: pager.counts}, nil)
	}
}

func readCopyRecordHeader(
	ctx context.Context,
	view *store.View,
	boardID domainboard.ID,
	storeOverrides configuration.Overrides,
	sourceView BackupSource,
	policy copyRecordPolicy,
) (boardcopy.RecordHeader, error) {
	var header boardcopy.RecordHeader
	if err := storeOverrides.Validate(); err != nil {
		return header, fmt.Errorf("source store configuration: %w", err)
	}
	if view == nil {
		return header, errors.New("source store view is required")
	}
	if strings.TrimSpace(sourceView.LineageID) == "" {
		return header, errors.New("source store lineage is required")
	}
	if sourceView.Revision < 0 {
		return header, errors.New("source store revision cannot be negative")
	}

	queries := query.New(view)
	source, err := queries.BoardGetCopySource(ctx, boardID.String())
	if errors.Is(err, sql.ErrNoRows) {
		return header, errkind.Errorf(errkind.NotFound, "board not found")
	}
	if err != nil {
		return header, fmt.Errorf("read source board: %w", err)
	}
	projectLayer, err := copyOverrides(
		source.ProjectIssueIDPrefix,
		source.ProjectIssueIDStrategy,
		source.ProjectIssueSummaryMaxBytes,
		source.ProjectAttachmentMaxBytes,
	)
	if err != nil {
		return header, fmt.Errorf("load source project configuration: %w", err)
	}
	boardLayer, err := copyOverrides(
		source.BoardIssueIDPrefix,
		source.BoardIssueIDStrategy,
		source.BoardIssueSummaryMaxBytes,
		source.BoardAttachmentMaxBytes,
	)
	if err != nil {
		return header, fmt.Errorf("load source board configuration: %w", err)
	}

	if policy == requireQuiescentRecords {
		activeClaims, err := queries.BoardCountCopyActiveClaims(ctx, boardID.String())
		if err != nil {
			return header, fmt.Errorf("inspect source claims: %w", err)
		}
		if activeClaims != 0 {
			return header, errkind.Errorf(
				errkind.Conflict,
				"source board has active claims",
			)
		}
		activeUploads, err := queries.AttachmentCountCopyActiveUploads(
			ctx,
			boardID.String(),
		)
		if err != nil {
			return header, fmt.Errorf("inspect source attachment uploads: %w", err)
		}
		if activeUploads != 0 {
			return header, errkind.Errorf(
				errkind.Conflict,
				"source board has active attachment uploads",
			)
		}
	}

	return boardcopy.RecordHeader{
		Version:         boardcopy.CopySnapshotVersion,
		SourceLineageID: sourceView.LineageID,
		SourceRevision:  sourceView.Revision,
		Board: boardcopy.CopyBoard{
			ID: source.BoardID, Name: source.BoardName,
			Description: source.BoardDescription, CreatedAt: source.BoardCreatedAt,
		},
		Configuration: resolveCopyConfiguration(
			storeOverrides,
			projectLayer,
			boardLayer,
		),
	}, nil
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

type copyRecordPager struct {
	ctx      context.Context // required
	queries  *query.Queries  // required
	boardID  string          // required
	pageSize int64
	counts   boardcopy.RecordCounts
}

func (p *copyRecordPager) read(
	yield func(boardcopy.Record, error) bool,
) bool {
	// boardcopy.RecordType defines and validates this section order. Keep the
	// repository readers explicit so each section retains its bounded keyset.
	return p.readIssues(yield) &&
		p.readLabels(yield) &&
		p.readDependencies(yield) &&
		p.readContainment(yield) &&
		p.readExternalKeys(yield) &&
		p.readLogEntries(yield) &&
		p.readStates(yield) &&
		p.readResults(yield) &&
		p.readCheckpoints(yield) &&
		p.readAttachments(yield)
}

func (p *copyRecordPager) readIssues(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterID string
	for {
		rows, err := p.queries.BoardListCopyIssuePage(
			p.ctx,
			query.BoardListCopyIssuePageParams{
				BoardID: p.boardID, AfterID: afterID, PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source issue page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			record := boardcopy.CopyIssue{
				ID: row.ID, Title: row.Title, Kind: row.Kind,
				Lifecycle: row.Lifecycle, Priority: row.Priority,
				CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
				ClosedAt: row.ClosedAt, WaitingReason: row.WaitingReason,
				WaitingSince: row.WaitingSince, Summary: row.Summary,
				Details: row.Details,
			}
			afterID = row.ID
			p.counts.Issues++
			if !yield(record, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readLabels(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterIssueID, afterLabel string
	for {
		rows, err := p.queries.BoardListCopyLabelPage(
			p.ctx,
			query.BoardListCopyLabelPageParams{
				BoardID: p.boardID, AfterIssueID: afterIssueID,
				AfterLabel: afterLabel, PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source issue-label page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterIssueID, afterLabel = row.IssueID, row.Label
			p.counts.Labels++
			if !yield(boardcopy.CopyLabel{
				IssueID: row.IssueID, Label: row.Label,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readDependencies(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterIssueID, afterPrerequisiteID string
	for {
		rows, err := p.queries.BoardListCopyDependencyPage(
			p.ctx,
			query.BoardListCopyDependencyPageParams{
				BoardID: p.boardID, AfterIssueID: afterIssueID,
				AfterPrerequisiteID: afterPrerequisiteID,
				PageSize:            p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source dependency page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterIssueID = row.IssueID
			afterPrerequisiteID = row.PrerequisiteID
			p.counts.Dependencies++
			if !yield(boardcopy.CopyDependency{
				IssueID: row.IssueID, PrerequisiteID: row.PrerequisiteID,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readContainment(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterChildID string
	for {
		rows, err := p.queries.BoardListCopyContainmentPage(
			p.ctx,
			query.BoardListCopyContainmentPageParams{
				BoardID: p.boardID, AfterChildID: afterChildID,
				PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source containment page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterChildID = row.ChildID
			p.counts.Containment++
			if !yield(boardcopy.CopyContainment{
				ChildID: row.ChildID, ParentID: row.ParentID,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readExternalKeys(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterKey, afterIssueID string
	for {
		rows, err := p.queries.BoardListCopyExternalKeyPage(
			p.ctx,
			query.BoardListCopyExternalKeyPageParams{
				BoardID: p.boardID, AfterExternalKey: afterKey,
				AfterIssueID: afterIssueID, PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source external-key page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterKey, afterIssueID = row.ExternalKey, row.IssueID
			p.counts.ExternalKeys++
			if !yield(boardcopy.CopyExternalKey{
				Key: row.ExternalKey, IssueID: row.IssueID,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readLogEntries(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterSequence int64
	for {
		rows, err := p.queries.BoardListCopyLogEntryPage(
			p.ctx,
			query.BoardListCopyLogEntryPageParams{
				BoardID: p.boardID, AfterLocalSequence: afterSequence,
				PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source Log-entry page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterSequence = row.LocalSequence
			record := boardcopy.CopyLogEntry{
				Order: p.counts.LogEntries, ID: row.ID, IssueID: row.IssueID,
				Kind: row.Kind, Author: row.Author, Committer: row.Committer,
				Body: row.Body, CreatedAt: row.CreatedAt,
				NextAction: row.NextAction,
			}
			p.counts.LogEntries++
			if !yield(record, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readStates(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterIssueID string
	for {
		rows, err := p.queries.BoardListCopyStatePage(
			p.ctx,
			query.BoardListCopyStatePageParams{
				BoardID: p.boardID, AfterIssueID: afterIssueID,
				PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source State page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterIssueID = row.IssueID
			p.counts.States++
			if !yield(boardcopy.CopyState{
				IssueID: row.IssueID, Body: row.Body, Author: row.Author,
				UpdatedAt:          row.UpdatedAt,
				SnapshotLogEntryID: row.SnapshotLogEntryID,
				NextAction:         row.NextAction,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readResults(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterIssueID string
	for {
		rows, err := p.queries.BoardListCopyResultPage(
			p.ctx,
			query.BoardListCopyResultPageParams{
				BoardID: p.boardID, AfterIssueID: afterIssueID,
				PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source Result page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterIssueID = row.IssueID
			p.counts.Results++
			if !yield(boardcopy.CopyResultRecord{
				IssueID: row.IssueID, Body: row.Body,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readCheckpoints(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterIssueID string
	for {
		rows, err := p.queries.BoardListCopyCheckpointPage(
			p.ctx,
			query.BoardListCopyCheckpointPageParams{
				BoardID: p.boardID, AfterIssueID: afterIssueID,
				PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source checkpoint page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			afterIssueID = row.IssueID
			p.counts.Checkpoints++
			if !yield(boardcopy.CopyCheckpoint{
				IssueID: row.IssueID, Outcome: row.Outcome,
				Reason: row.Reason, DecidedAt: row.DecidedAt,
			}, nil) {
				return false
			}
		}
	}
}

func (p *copyRecordPager) readAttachments(
	yield func(boardcopy.Record, error) bool,
) bool {
	var afterID string
	for {
		rows, err := p.queries.AttachmentListCopyMetadataPage(
			p.ctx,
			query.AttachmentListCopyMetadataPageParams{
				BoardID: p.boardID, AfterID: afterID, PageSize: p.pageSize,
			},
		)
		if err != nil {
			yield(nil, fmt.Errorf("read source attachment page: %w", err))
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, row := range rows {
			digest, err := attachment.NewDigest(row.BlobDigest)
			if err != nil {
				yield(nil, fmt.Errorf("load source attachment digest: %w", err))
				return false
			}
			blob := attachment.BlobDescriptor{
				Digest: digest, SizeBytes: uint64(row.BlobSizeBytes),
			}
			if err := blob.Validate(); err != nil {
				yield(nil, fmt.Errorf("load source attachment blob: %w", err))
				return false
			}
			afterID = row.ID
			p.counts.Attachments++
			if !yield(boardcopy.CopyAttachment{
				ID: row.ID, OriginIssueID: row.OriginIssueID, Blob: blob,
				Filename: row.Filename, MediaType: row.MediaType,
				Lifecycle: row.Lifecycle, CreatedActor: row.CreatedActor,
				CreatedAt: row.CreatedAt, RemovedActor: row.RemovedActor,
				RemovedAt: row.RemovedAt,
			}, nil) {
				return false
			}
		}
	}
}
