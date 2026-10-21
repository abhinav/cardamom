// Package boardhistory persists append-only semantic history beside the board
// projection owned by other repository packages.
package boardhistory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"iter"
	"strconv"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	history "go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// InitializeV1 streams one board's current shared projection into the first
// mutation of its local writer chain. The caller must provide a transaction
// that also owns the projection and board replica identity. This function and
// its BoardHistory queries are the frozen adapter for migration format V1;
// later formats must add an initializer instead of changing these semantics.
func InitializeV1(
	ctx context.Context,
	db query.DBTX,
	boardID board.ID,
	observedAt time.Time,
) error {
	if observedAt.IsZero() {
		return errors.New("initialize board history: observation time required")
	}
	queries := query.New(db)
	boardRow, err := queries.BoardHistoryGetBoardBaseline(ctx, boardID.String())
	if err != nil {
		return fmt.Errorf("initialize board history: read board: %w", err)
	}
	writer, err := board.ParseWriterUID(boardRow.WriterUid)
	if err != nil {
		return fmt.Errorf("initialize board history: %w", err)
	}
	reader := &baselineReader{
		ctx: ctx, queries: queries, boardID: boardID.String(),
		observedAt: observedAt.UTC(), board: boardRow,
	}
	header := history.Header{
		Board:    boardID,
		Writer:   writer,
		Sequence: 1,
	}
	marker, err := history.EncodeMutation(
		header,
		reader.events(),
		func(part history.Part) error {
			return queries.BoardHistoryInsertPart(
				ctx,
				query.BoardHistoryInsertPartParams{
					BoardID: boardID.String(), WriterUid: writer.Bytes(),
					Sequence: 1, PartIndex: part.Index, Bytes: part.Bytes,
				},
			)
		},
	)
	if err != nil {
		return fmt.Errorf("initialize board history: encode baseline: %w", err)
	}
	if err := queries.BoardHistoryInsertCommit(
		ctx,
		query.BoardHistoryInsertCommitParams{
			BoardID: boardID.String(), WriterUid: writer.Bytes(), Sequence: 1,
			PreviousHash: marker.Header().Previous.Bytes(),
			MarkerHash:   marker.Hash().Bytes(), MarkerBytes: marker.Bytes(),
			PartCount: marker.PartCount(), EventCount: int64(marker.EventCount()),
			PayloadHash: marker.PayloadHash().Bytes(),
		},
	); err != nil {
		return fmt.Errorf("initialize board history: insert terminal marker: %w", err)
	}
	if err := retainAttachmentBlobs(ctx, queries, boardID, writer); err != nil {
		return err
	}
	if err := queries.BoardHistoryInsertTip(
		ctx,
		query.BoardHistoryInsertTipParams{
			BoardID: boardID.String(), WriterUid: writer.Bytes(),
			Sequence: 1, MarkerHash: marker.Hash().Bytes(),
		},
	); err != nil {
		return fmt.Errorf("initialize board history: insert writer tip: %w", err)
	}
	return nil
}

type baselineReader struct {
	ctx        context.Context
	queries    *query.Queries
	boardID    string
	observedAt time.Time
	board      query.BoardHistoryGetBoardBaselineRow
}

func (r *baselineReader) events() iter.Seq2[history.Event, error] {
	return func(yield func(history.Event, error) bool) {
		boardEvent, err := r.boardEvent()
		if err != nil {
			yield(history.Event{OccurredAt: time.Time{}, Payload: nil}, err)
			return
		}
		if !yield(boardEvent, nil) {
			return
		}
		if !r.issueEvents(yield) || !r.labelEvents(yield) ||
			!r.dependencyEvents(yield) || !r.containmentEvents(yield) ||
			!r.externalKeyEvents(yield) || !r.pinEvents(yield) {
			return
		}
		r.attachmentEvents(yield)
	}
}

func (r *baselineReader) boardEvent() (history.Event, error) {
	createdAt, err := parseStoredTime(r.board.CreatedAtText)
	if err != nil {
		return history.Event{}, fmt.Errorf("read board creation time: %w", err)
	}
	archivedAt, err := parseOptionalStoredTime(r.board.ArchivedAtText)
	if err != nil {
		return history.Event{}, fmt.Errorf("read board archive time: %w", err)
	}
	var archive *board.Archive
	if archivedAt != nil && r.board.ArchivedBy != nil {
		archive = &board.Archive{
			Actor:  *r.board.ArchivedBy,
			At:     *archivedAt,
			Reason: r.board.ArchiveReason,
		}
	}
	return history.Event{
		OccurredAt: r.observedAt,
		Payload: &history.BoardOrigin{
			CreatedAt: createdAt, Name: r.board.Name,
			Description: r.board.Description, Archive: archive,
			IssueIDPrefix:      r.board.IssueIDPrefix,
			IssueIDStrategy:    r.board.IssueIDStrategy,
			SummaryMaxBytes:    r.board.IssueSummaryMaxBytes,
			AttachmentMaxBytes: r.board.AttachmentMaxBytes,
			PinsMaxCount:       r.board.BoardPinsMaxCount,
		},
	}, nil
}

func (r *baselineReader) issueEvents(yield func(history.Event, error) bool) bool {
	afterUID := []byte{}
	for {
		row, err := r.queries.BoardHistoryNextIssue(
			r.ctx,
			query.BoardHistoryNextIssueParams{BoardID: r.boardID, AfterUid: afterUID},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next issue", err)
		}
		uid, err := issue.ParseUID(row.Uid)
		if err != nil {
			return yieldError(yield, "read issue identity", err)
		}
		createdAt, err := parseStoredTime(row.CreatedAtText)
		if err != nil {
			return yieldError(yield, "read issue creation time", err)
		}
		updatedAt, err := parseStoredTime(row.UpdatedAtText)
		if err != nil {
			return yieldError(yield, "read issue update time", err)
		}
		closedAt, err := parseOptionalStoredTime(row.ClosedAtText)
		if err != nil {
			return yieldError(yield, "read issue closure time", err)
		}
		waitingSince, err := parseOptionalStoredTime(row.WaitingSinceText)
		if err != nil {
			return yieldError(yield, "read issue waiting time", err)
		}
		if (row.WaitingReason == nil) != (waitingSince == nil) {
			return yieldError(yield, "read issue waiting state", errors.New("incomplete waiting state"))
		}
		var waiting *issue.WaitingState
		if row.WaitingReason != nil && waitingSince != nil {
			waiting = &issue.WaitingState{
				Reason: *row.WaitingReason,
				Since:  *waitingSince,
			}
		}
		origin := &history.IssueOrigin{
			UID: uid, PublicID: row.ID, Title: row.Title,
			Kind: row.Kind, Lifecycle: row.Lifecycle, Priority: row.Priority,
			CreatedAt: createdAt, UpdatedAt: updatedAt,
			ClosedAt: closedAt, Waiting: waiting, Summary: row.Summary,
			Details: row.Details,
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: origin}, nil) ||
			!r.logEvents(uid, yield) {
			return false
		}
		if row.StateBody != nil {
			stateUpdatedAt, err := parseOptionalStoredTime(row.StateUpdatedAtText)
			if err != nil {
				return yieldError(yield, "read State update time", err)
			}
			at := r.observedAt
			if stateUpdatedAt != nil {
				at = *stateUpdatedAt
			}
			if !yield(history.Event{OccurredAt: at, Payload: &history.IssueState{
				IssueUID: uid,
				State: &history.StateRecord{
					Body: *row.StateBody, Author: row.StateAuthor,
					UpdatedAt:          stateUpdatedAt,
					SnapshotLogEntryID: row.SnapshotLogEntryID,
					NextAction:         row.StateNextAction,
				},
			}}, nil) {
				return false
			}
		}
		if row.ResultBody != nil && !yield(history.Event{
			OccurredAt: r.observedAt,
			Payload:    &history.IssueResult{IssueUID: uid, Body: row.ResultBody},
		}, nil) {
			return false
		}
		if row.CheckpointOutcome != nil {
			checkpointDecidedAt, err := parseOptionalStoredTime(row.CheckpointDecidedAtText)
			if err != nil {
				return yieldError(yield, "read checkpoint decision time", err)
			}
			if row.CheckpointReason == nil || checkpointDecidedAt == nil {
				return yieldError(yield, "read checkpoint", errors.New("incomplete decision"))
			}
			if !yield(history.Event{
				OccurredAt: *checkpointDecidedAt,
				Payload: &history.IssueCheckpoint{
					IssueUID: uid, Outcome: *row.CheckpointOutcome,
					Reason: *row.CheckpointReason, DecidedAt: *checkpointDecidedAt,
				},
			}, nil) {
				return false
			}
		}
		afterUID = append(afterUID[:0], row.Uid...)
	}
}

func (r *baselineReader) logEvents(
	uid issue.UID,
	yield func(history.Event, error) bool,
) bool {
	var after int64
	for {
		row, err := r.queries.BoardHistoryNextLog(
			r.ctx,
			query.BoardHistoryNextLogParams{
				BoardID: r.boardID, IssueUid: uid.Bytes(), AfterSequence: after,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next Log entry", err)
		}
		createdAt, err := parseOptionalStoredTime(row.CreatedAtText)
		if err != nil {
			return yieldError(yield, "read Log entry creation time", err)
		}
		at := r.observedAt
		if createdAt != nil {
			at = *createdAt
		}
		if !yield(history.Event{OccurredAt: at, Payload: &history.IssueLog{
			IssueUID: uid, ID: row.ID, Kind: row.Kind,
			Author: row.Author, Committer: row.Committer, Body: row.Body,
			CreatedAt: createdAt, NextAction: row.NextAction,
		}}, nil) {
			return false
		}
		after = row.LocalSequence
	}
}

func (r *baselineReader) labelEvents(yield func(history.Event, error) bool) bool {
	afterUID := []byte{}
	var afterLabel string
	for {
		row, err := r.queries.BoardHistoryNextLabel(
			r.ctx,
			query.BoardHistoryNextLabelParams{
				BoardID: r.boardID, AfterUid: afterUID, AfterLabel: afterLabel,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next issue label", err)
		}
		uid, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return yieldError(yield, "read label issue identity", err)
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: &history.IssueLabel{
			IssueUID: uid, Label: row.Label, Present: true,
		}}, nil) {
			return false
		}
		afterUID = append(afterUID[:0], row.IssueUid...)
		afterLabel = row.Label
	}
}

func (r *baselineReader) dependencyEvents(yield func(history.Event, error) bool) bool {
	afterUID := []byte{}
	afterPrerequisite := []byte{}
	for {
		row, err := r.queries.BoardHistoryNextDependency(
			r.ctx,
			query.BoardHistoryNextDependencyParams{
				BoardID: r.boardID, AfterUid: afterUID,
				AfterPrerequisiteUid: afterPrerequisite,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next dependency", err)
		}
		issueUID, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return yieldError(yield, "read dependency issue identity", err)
		}
		prerequisiteUID, err := issue.ParseUID(row.PrerequisiteUid)
		if err != nil {
			return yieldError(yield, "read prerequisite identity", err)
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: &history.IssueDependency{
			IssueUID: issueUID, PrerequisiteUID: prerequisiteUID, Present: true,
		}}, nil) {
			return false
		}
		afterUID = append(afterUID[:0], row.IssueUid...)
		afterPrerequisite = append(afterPrerequisite[:0], row.PrerequisiteUid...)
	}
}

func (r *baselineReader) containmentEvents(yield func(history.Event, error) bool) bool {
	afterUID := []byte{}
	for {
		row, err := r.queries.BoardHistoryNextContainment(
			r.ctx,
			query.BoardHistoryNextContainmentParams{BoardID: r.boardID, AfterUid: afterUID},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next containment edge", err)
		}
		childUID, err := issue.ParseUID(row.ChildUid)
		if err != nil {
			return yieldError(yield, "read child identity", err)
		}
		parentUID, err := issue.ParseUID(row.ParentUid)
		if err != nil {
			return yieldError(yield, "read parent identity", err)
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: &history.IssueContainment{
			ChildUID: childUID, ParentUID: parentUID, Present: true,
		}}, nil) {
			return false
		}
		afterUID = append(afterUID[:0], row.ChildUid...)
	}
}

func (r *baselineReader) externalKeyEvents(yield func(history.Event, error) bool) bool {
	var afterKey string
	for {
		row, err := r.queries.BoardHistoryNextExternalKey(
			r.ctx,
			query.BoardHistoryNextExternalKeyParams{BoardID: r.boardID, AfterKey: afterKey},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next external key", err)
		}
		uid, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return yieldError(yield, "read external-key issue identity", err)
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: &history.ExternalKey{
			Key: row.ExternalKey, IssueUID: uid, Present: true,
		}}, nil) {
			return false
		}
		afterKey = row.ExternalKey
	}
}

func (r *baselineReader) pinEvents(yield func(history.Event, error) bool) bool {
	afterUID := []byte{}
	for {
		row, err := r.queries.BoardHistoryNextPin(
			r.ctx,
			query.BoardHistoryNextPinParams{BoardID: r.boardID, AfterUid: afterUID},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next board pin", err)
		}
		uid, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return yieldError(yield, "read pinned issue identity", err)
		}
		if !yield(history.Event{OccurredAt: r.observedAt, Payload: &history.BoardPin{
			IssueUID: uid, Position: row.Position, Present: true,
		}}, nil) {
			return false
		}
		afterUID = append(afterUID[:0], row.IssueUid...)
	}
}

func (r *baselineReader) attachmentEvents(yield func(history.Event, error) bool) bool {
	var afterID string
	for {
		row, err := r.queries.BoardHistoryNextAttachment(
			r.ctx,
			query.BoardHistoryNextAttachmentParams{BoardID: r.boardID, AfterID: afterID},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return true
		}
		if err != nil {
			return yieldError(yield, "read next attachment", err)
		}
		var origin *issue.UID
		if row.OriginIssueUid != nil {
			parsed, err := issue.ParseUID(row.OriginIssueUid)
			if err != nil {
				return yieldError(yield, "read attachment origin identity", err)
			}
			origin = &parsed
		}
		createdAt, err := parseStoredTime(row.CreatedAtText)
		if err != nil {
			return yieldError(yield, "read attachment creation time", err)
		}
		if !yield(history.Event{OccurredAt: createdAt, Payload: &history.AttachmentAdd{
			ID: row.ID, OriginIssueUID: origin, Digest: row.BlobDigest,
			SizeBytes: row.BlobSizeBytes, Filename: row.Filename,
			MediaType: row.MediaType, Actor: row.CreatedActor, At: createdAt,
		}}, nil) {
			return false
		}
		if row.Lifecycle == "removed" {
			removedAt, err := parseOptionalStoredTime(row.RemovedAtText)
			if err != nil {
				return yieldError(yield, "read attachment removal time", err)
			}
			if row.RemovedActor == nil || removedAt == nil {
				return yieldError(yield, "read attachment removal", errors.New("incomplete tombstone"))
			}
			if !yield(history.Event{OccurredAt: *removedAt, Payload: &history.AttachmentRemove{
				ID: row.ID, Actor: *row.RemovedActor, At: *removedAt,
			}}, nil) {
				return false
			}
		}
		afterID = row.ID
	}
}

func retainAttachmentBlobs(
	ctx context.Context,
	queries *query.Queries,
	boardID board.ID,
	writer board.WriterUID,
) error {
	var afterID string
	for {
		row, err := queries.BoardHistoryNextAttachment(
			ctx,
			query.BoardHistoryNextAttachmentParams{
				BoardID: boardID.String(), AfterID: afterID,
			},
		)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("initialize board history: read attachment reference: %w", err)
		}
		if err := queries.BoardHistoryInsertBlobReference(
			ctx,
			query.BoardHistoryInsertBlobReferenceParams{
				BoardID: boardID.String(), WriterUid: writer.Bytes(), Sequence: 1,
				Digest: row.BlobDigest, SizeBytes: row.BlobSizeBytes,
			},
		); err != nil {
			return fmt.Errorf("initialize board history: retain attachment blob: %w", err)
		}
		afterID = row.ID
	}
}

func yieldError(
	yield func(history.Event, error) bool,
	operation string,
	err error,
) bool {
	yield(
		history.Event{OccurredAt: time.Time{}, Payload: nil},
		fmt.Errorf("%s: %w", operation, err),
	)
	return false
}

func parseOptionalStoredTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := parseStoredTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseStoredTime(value string) (time.Time, error) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(seconds, 0).UTC(), nil
	}
	formats := [...]string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02T15:04:05.999999999",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"2006-01-02",
	}
	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported stored time %q", value)
}
