package board

import (
	"context"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) writeConflictIssueValue(ctx context.Context, queries *query.Queries, unit boardconflict.Unit, payload boardhistory.Payload, changedAt *time.Time, revision int64) error {
	switch unit.Kind {
	case boardhistory.EventIssueTitleSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.IssueTitleSet)
			current = new(value.Title)
		}
		return queries.BoardSetIssueTitleProjection(ctx, query.BoardSetIssueTitleProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Value: current, UpdatedAt: changedAt})
	case boardhistory.EventIssueKindSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.IssueKindSet)
			current = new(value.Kind)
		}
		return queries.BoardSetIssueKindProjection(ctx, query.BoardSetIssueKindProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Value: current, UpdatedAt: changedAt})
	case boardhistory.EventIssuePrioritySet:
		var current *int64
		if payload != nil {
			value := payload.(*boardhistory.IssuePrioritySet)
			current = new(value.Priority)
		}
		return queries.BoardSetIssuePriorityProjection(ctx, query.BoardSetIssuePriorityProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Value: current, UpdatedAt: changedAt})
	case boardhistory.EventIssueSummarySet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.IssueSummarySet)
			current = value.Summary
		}
		return queries.BoardSetIssueSummaryProjection(ctx, query.BoardSetIssueSummaryProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Value: current, UpdatedAt: changedAt})
	case boardhistory.EventIssueDetailsSet:
		var current *string
		if payload != nil {
			value := payload.(*boardhistory.IssueDetailsSet)
			current = value.Details
		}
		return queries.BoardSetIssueDetailsProjection(ctx, query.BoardSetIssueDetailsProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Value: current, UpdatedAt: changedAt})
	case boardhistory.EventIssueLifecycleSet:
		var lifecycle *string
		var closedAt *time.Time
		if payload != nil {
			value := payload.(*boardhistory.IssueLifecycleSet)
			lifecycle, closedAt = &value.Lifecycle, value.ClosedAt
		}
		return queries.BoardSetIssueLifecycleProjection(ctx, query.BoardSetIssueLifecycleProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Lifecycle: lifecycle, ClosedAt: closedAt, UpdatedAt: changedAt})
	case boardhistory.EventIssueWaitingSet:
		var reason *string
		var since *time.Time
		if payload != nil {
			value := payload.(*boardhistory.IssueWaitingSet)
			if value.Waiting != nil {
				reason, since = &value.Waiting.Reason, &value.Waiting.Since
			}
		}
		return queries.BoardSetIssueWaitingProjection(ctx, query.BoardSetIssueWaitingProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Reason: reason, Since: since, UpdatedAt: changedAt})
	case boardhistory.EventIssueState:
		if payload == nil || payload.(*boardhistory.IssueState).State == nil {
			return queries.BoardDeleteIssueState(ctx, query.BoardDeleteIssueStateParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes()})
		}
		state := payload.(*boardhistory.IssueState).State
		return queries.BoardUpsertIssueState(ctx, query.BoardUpsertIssueStateParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Body: state.Body, NextAction: state.NextAction, Author: state.Author, UpdatedAt: state.UpdatedAt, SnapshotLogEntryID: state.SnapshotLogEntryID})
	case boardhistory.EventIssueResult:
		if payload == nil || payload.(*boardhistory.IssueResult).Body == nil {
			return queries.BoardDeleteIssueResultProjection(ctx, query.BoardDeleteIssueResultProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes()})
		}
		return queries.BoardUpsertIssueResult(ctx, query.BoardUpsertIssueResultParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Body: *payload.(*boardhistory.IssueResult).Body})
	case boardhistory.EventIssueCheckpoint:
		if err := queries.BoardDeleteCheckpointProjection(ctx, query.BoardDeleteCheckpointProjectionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes()}); err != nil {
			return err
		}
		if payload == nil {
			return nil
		}
		value := payload.(*boardhistory.IssueCheckpoint)
		return queries.BoardInsertCheckpointDecision(ctx, query.BoardInsertCheckpointDecisionParams{BoardID: r.boardID.String(), IssueUid: unit.IssueUID.Bytes(), Outcome: value.Outcome, Reason: value.Reason, DecidedAt: value.DecidedAt, Revision: revision})
	default:
		return fmt.Errorf("unsupported issue projection %s", unit.Kind)
	}
}
