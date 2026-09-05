package board

import (
	"context"
	"database/sql"
	"errors"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// SetState replaces one issue's mutable recovery State.
func (r *Repository) SetState(
	ctx context.Context,
	command record.SetState,
) (out record.StateSet, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.SetState(command)
	if err != nil {
		return out, err
	}
	out.Issue, err = r.commitState(ctx, mutation, out.Issue)
	if err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

// ClearState removes one issue's mutable recovery State.
func (r *Repository) ClearState(
	ctx context.Context,
	command record.ClearState,
) (out record.StateSet, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.ClearState(command)
	if err != nil {
		return out, err
	}
	out.Issue, err = r.commitState(ctx, mutation, out.Issue)
	if err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

// AppendState appends one paragraph to an issue's mutable recovery state.
func (r *Repository) AppendState(
	ctx context.Context,
	command record.AppendState,
) (out record.StateAppended, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.AppendState(command)
	if err != nil || !out.Changed {
		return out, err
	}
	out.Issue, err = r.commitState(ctx, mutation, out.Issue)
	if err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

func (r *Repository) commitState(
	ctx context.Context,
	mutation *mutation,
	state issue.State,
) (issue.State, error) {
	if err := mutation.reserve(ctx); err != nil {
		return state, err
	}
	state, err := r.persistIssueState(ctx, mutation, state)
	if err != nil {
		return state, err
	}
	return state, mutation.commit(ctx, state.ID())
}

// persistIssueState writes the issue projection and its optional attributed
// recovery State.
func (r *Repository) persistIssueState(
	ctx context.Context,
	mutation *mutation,
	state issue.State,
) (issue.State, error) {
	recovery := state.RecoveryStateRecord()
	if err := r.updateIssue(ctx, mutation, state); err != nil {
		return state, err
	}
	queries := query.New(mutation.change)
	if recovery == nil {
		err := queries.BoardDeleteIssueState(
			ctx,
			query.BoardDeleteIssueStateParams{
				BoardID: r.boardID.String(),
				IssueID: state.ID().String(),
			},
		)
		return state, err
	}
	err := queries.BoardUpsertIssueState(
		ctx,
		query.BoardUpsertIssueStateParams{
			IssueID:    state.ID().String(),
			BoardID:    r.boardID.String(),
			Body:       recovery.Body,
			NextAction: optionalString(recovery.NextAction),
			Author:     optionalActorString(recovery.Author),
			UpdatedAt:  recovery.UpdatedAt,
			SnapshotLogEntryID: optionalLogIDString(
				recovery.SnapshotLogEntryID,
			),
		},
	)
	return state, err
}

// logEntryWrite is the repository-private row shape for immutable chronology.
type logEntryWrite struct {
	issueID    issue.ID
	kind       issue.LogEntryKind
	author     issue.Actor
	committer  issue.Actor
	body       string
	nextAction string
	created    *time.Time
}

// insertLogEntry writes one log row and returns its generated identity.
func (r *Repository) insertLogEntry(
	ctx context.Context,
	mutation *mutation,
	entry logEntryWrite,
) (issue.LogID, error) {
	id, err := newLogID(r.entropy)
	if err != nil {
		return "", err
	}
	err = query.New(mutation.change).BoardInsertIssueLogEntry(
		ctx,
		query.BoardInsertIssueLogEntryParams{
			ID:         id.String(),
			BoardID:    r.boardID.String(),
			IssueID:    entry.issueID.String(),
			Kind:       entry.kind.String(),
			Author:     optionalActorString(entry.author),
			Committer:  optionalActorString(entry.committer),
			Body:       entry.body,
			NextAction: optionalString(entry.nextAction),
			CreatedAt:  entry.created,
		},
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// AddLogEntry appends one immutable attributed issue record.
func (r *Repository) AddLogEntry(
	ctx context.Context,
	command record.AddLogEntry,
) (out record.LogEntryAdded, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.AddLogEntry(command)
	if err != nil {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	id, err := r.insertLogEntry(ctx, mutation, logEntryWrite{
		issueID:    out.LogEntry.IssueID,
		kind:       out.LogEntry.Kind,
		author:     out.LogEntry.Author,
		committer:  out.LogEntry.Committer,
		body:       out.LogEntry.Body,
		nextAction: out.LogEntry.NextAction,
		created:    out.LogEntry.Created,
	})
	if err != nil {
		return out, err
	}
	out.LogEntry.ID = id
	if err := mutation.commit(ctx, out.LogEntry.IssueID); err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

// CommitState snapshots changed State and applies its selected disposition.
func (r *Repository) CommitState(
	ctx context.Context,
	command record.CommitState,
) (out record.StateCommitted, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.CommitState(command)
	if err != nil || !out.Changed {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	if out.LogEntry != nil {
		id, err := r.insertLogEntry(ctx, mutation, logEntryWrite{
			issueID:    out.LogEntry.IssueID,
			kind:       out.LogEntry.Kind,
			author:     out.LogEntry.Author,
			committer:  out.LogEntry.Committer,
			body:       out.LogEntry.Body,
			nextAction: out.LogEntry.NextAction,
			created:    out.LogEntry.Created,
		})
		if err != nil {
			return out, err
		}
		out.LogEntry.ID = id
		if command.Disposition == record.CommitStateRetain {
			out.Issue = out.Issue.WithRecoveryStateSnapshot(&id)
		}
	}
	out.Issue, err = r.persistIssueState(ctx, mutation, out.Issue)
	if err != nil {
		return out, err
	}
	if err := mutation.commit(ctx, out.Issue.ID()); err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

// SetResult replaces one issue's current durable outcome.
func (r *Repository) SetResult(
	ctx context.Context,
	command record.SetResult,
) (out record.ResultSet, err error) {
	mutation, board, err := r.recordMutation(ctx, command.IssueID)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, mutation.change.Done()) }()
	out, err = board.SetResult(command)
	if err != nil {
		return out, err
	}
	if err := mutation.reserve(ctx); err != nil {
		return out, err
	}
	if err := query.New(mutation.change).BoardUpsertIssueResult(
		ctx,
		query.BoardUpsertIssueResultParams{
			IssueID: out.IssueID.String(),
			BoardID: r.boardID.String(),
			Body:    out.Body,
		},
	); err != nil {
		return out, err
	}
	if err := mutation.commit(ctx, out.IssueID); err != nil {
		return out, err
	}
	out.CommittedRevision = recordCommittedRevision(mutation)
	return out, nil
}

type recordPolicy interface {
	SetState(record.SetState) (record.StateSet, error)
	ClearState(record.ClearState) (record.StateSet, error)
	AppendState(record.AppendState) (record.StateAppended, error)
	AddLogEntry(record.AddLogEntry) (record.LogEntryAdded, error)
	CommitState(record.CommitState) (record.StateCommitted, error)
	SetResult(record.SetResult) (record.ResultSet, error)
}

func (r *Repository) recordMutation(
	ctx context.Context,
	id issue.ID,
) (*mutation, recordPolicy, error) {
	mutation, err := r.beginMutation(ctx)
	if err != nil {
		return nil, nil, err
	}
	state, _, err := r.readIssueState(ctx, mutation.change, id)
	if err != nil {
		return nil, nil, errors.Join(err, mutation.change.Done())
	}
	policy, err := record.Load(record.Snapshot{
		BoardID: r.boardID, Revision: mutation.current, Issue: state, OccurredAt: mutation.occurredAt,
	})
	if err != nil {
		return nil, nil, errors.Join(err, mutation.change.Done())
	}
	return mutation, policy, nil
}

func recordCommittedRevision(mutation *mutation) record.CommittedRevision {
	return record.CommittedRevision{
		Revision: domainboard.Revision(mutation.reservation.Revision()),
	}
}

// ListLogEntries reads one issue's immutable log entries in durable order.
func (r *Repository) ListLogEntries(ctx context.Context, request issue.LogListRequest) (out []issue.LogEntry, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return nil, err
	}
	exists, err := r.issueExists(ctx, view, id)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errkind.Errorf(errkind.NotFound, "issue not found: %s", id)
	}
	limit := int64(-1)
	if request.Limit > 0 {
		limit = int64(request.Limit)
	}
	out = make([]issue.LogEntry, 0)
	queries := query.New(view)
	if request.Reverse {
		rows, err := queries.BoardListIssueLogEntriesDescending(
			ctx,
			query.BoardListIssueLogEntriesDescendingParams{
				BoardID: r.boardID.String(), IssueID: id.String(), LimitCount: limit,
			},
		)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			entry, err := newIssueLogEntry(
				row.ID,
				row.IssueID,
				row.Kind,
				row.Author,
				row.Committer,
				row.Body,
				row.NextAction,
				row.CreatedAt,
			)
			if err != nil {
				return nil, err
			}
			out = append(out, entry)
		}
		return out, nil
	}
	rows, err := queries.BoardListIssueLogEntriesAscending(
		ctx,
		query.BoardListIssueLogEntriesAscendingParams{
			BoardID: r.boardID.String(), IssueID: id.String(), LimitCount: limit,
		},
	)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		entry, err := newIssueLogEntry(
			row.ID,
			row.IssueID,
			row.Kind,
			row.Author,
			row.Committer,
			row.Body,
			row.NextAction,
			row.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// ReadLogEntry reads one immutable Log entry by its stable ID.
func (r *Repository) ReadLogEntry(
	ctx context.Context,
	request record.GetLogEntryRequest,
) (out issue.LogEntry, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	logID, err := issue.NewLogID(request.LogID)
	if err != nil {
		return out, err
	}
	row, err := query.New(view).BoardReadIssueLogEntry(
		ctx,
		query.BoardReadIssueLogEntryParams{
			BoardID: r.boardID.String(),
			LogID:   logID.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return out, errkind.Errorf(errkind.NotFound, "log entry not found: %s", logID)
	}
	if err != nil {
		return out, err
	}
	return newIssueLogEntry(
		row.ID,
		row.IssueID,
		row.Kind,
		row.Author,
		row.Committer,
		row.Body,
		row.NextAction,
		row.CreatedAt,
	)
}

func newIssueLogEntry(
	logID, issueID, kind string,
	author, committer *string,
	body string,
	nextAction *string,
	created *time.Time,
) (issue.LogEntry, error) {
	parsed, err := issue.NewLogID(logID)
	if err != nil {
		return issue.LogEntry{}, err
	}
	parsedKind, err := issue.NewLogEntryKind(kind)
	if err != nil {
		return issue.LogEntry{}, err
	}
	return issue.LogEntry{
		ID: parsed, IssueID: issueID, Kind: parsedKind.String(),
		Author: author, Committer: committer, Body: body,
		NextAction: nextAction,
		Created:    optionalUnixTime(created),
	}, nil
}

func optionalActorString(actor issue.Actor) *string {
	if actor == "" {
		return nil
	}
	return new(actor.String())
}

func optionalLogIDString(id *issue.LogID) *string {
	if id == nil {
		return nil
	}
	return new(id.String())
}

func optionalUnixTime(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	return new(value.Unix())
}

// ReadResult reads one issue's current durable result.
func (r *Repository) ReadResult(ctx context.Context, request issue.ResultRequest) (out issue.Result, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return out, err
	}
	row, err := query.New(view).BoardReadIssueResult(
		ctx,
		query.BoardReadIssueResultParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		exists, existsErr := r.issueExists(ctx, view, id)
		if existsErr != nil {
			return out, existsErr
		}
		if exists {
			return out, errkind.Errorf(errkind.NotFound, "issue result not found")
		}
		return out, errkind.Errorf(errkind.NotFound, "issue not found: %s", id)
	}
	if err != nil {
		return out, err
	}
	return issue.Result{IssueID: row.ID, Title: row.Title, Body: row.Body}, nil
}
