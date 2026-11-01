package board

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

func (r *Repository) insertIssue(ctx context.Context, mutation *mutation, state issue.State) error {
	if err := state.RequireResolved(issue.FieldTitle, issue.FieldKind, issue.FieldLifecycle, issue.FieldPriority); err != nil {
		return err
	}
	snapshot := state.Snapshot()
	if err := query.New(mutation.change).BoardInsertIssue(
		ctx,
		query.BoardInsertIssueParams{
			Uid:           snapshot.UID.Bytes(),
			ID:            snapshot.ID.String(),
			BoardID:       r.boardID.String(),
			Title:         snapshot.Title,
			Kind:          kindText(snapshot.Kind),
			Lifecycle:     lifecycleText(snapshot.Lifecycle),
			Priority:      priorityInteger(snapshot.Priority),
			CreatedAt:     snapshot.Created,
			UpdatedAt:     snapshot.Updated,
			ClosedAt:      snapshot.ClosedAt,
			WaitingReason: waitingReason(snapshot.Waiting),
			WaitingSince:  waitingSince(snapshot.Waiting),
			Summary:       nullableText(snapshot.Summary),
			Details:       nullableText(snapshot.Details),
		},
	); err != nil {
		return err
	}
	mutation.record(issueOriginEvent(state))
	return nil
}

func (r *Repository) updateIssue(ctx context.Context, mutation *mutation, state issue.State) error {
	before, _, err := r.readIssueState(ctx, mutation.change, state.ID())
	if err != nil {
		return err
	}
	snapshot := state.Snapshot()
	if err := query.New(mutation.change).BoardUpdateIssue(
		ctx,
		query.BoardUpdateIssueParams{
			Title:         snapshot.Title,
			Kind:          kindText(snapshot.Kind),
			Lifecycle:     lifecycleText(snapshot.Lifecycle),
			Priority:      priorityInteger(snapshot.Priority),
			UpdatedAt:     snapshot.Updated,
			ClosedAt:      snapshot.ClosedAt,
			WaitingReason: waitingReason(snapshot.Waiting),
			WaitingSince:  waitingSince(snapshot.Waiting),
			Summary:       nullableText(snapshot.Summary),
			Details:       nullableText(snapshot.Details),
			BoardID:       r.boardID.String(),
			Uid:           snapshot.UID.Bytes(),
		},
	); err != nil {
		return err
	}
	recordIssueChanges(mutation, before, state)
	return nil
}

// persistedIssue is the Board repository representation of one issue row and
// its joined active claim and current result.
type persistedIssue struct {
	// uid, id, title, kind, lifecycle, and priority come from the issue row.
	uid       []byte
	id        string
	title     *string
	kind      *string
	lifecycle *string
	priority  *int64

	// createdAt and updatedAt come from the issue row.
	createdAt time.Time
	updatedAt time.Time

	// closedAt, waitingReason, and waitingSince come from the issue row.
	closedAt      *time.Time
	waitingReason *string
	waitingSince  *time.Time

	// summary and details are optional stable issue records.
	summary *string
	details *string

	// State fields come from the optional mutable recovery record.
	stateBody       *string
	stateNextAction *string
	stateAuthor     *string
	stateUpdatedAt  *time.Time
	stateSnapshotID *string

	// revision is the scalar issue revision observed with the issue row.
	revision int64

	// claimActor and claimStartedAt come from the optional active-claim join.
	claimActor     *string
	claimStartedAt *time.Time

	// resultBody comes from the optional current-result join.
	resultBody *string
}

// issueStateAtRevision retains one loaded issue and its scalar revision from
// the same repository snapshot.
type issueStateAtRevision struct {
	state    issue.State
	revision domainboard.Revision
}

// boardIssueIndex retains full records for existing detail and content-list
// readers. Presentation and eligibility use the same compact index as claim
// selection; only this content-bearing reader loads full issue states.
type boardIssueIndex struct {
	states       map[issue.ID]issueStateAtRevision
	presentation *issuePresentation
}

func (idx boardIssueIndex) summary(id issue.ID) issue.Summary {
	value := idx.presentation.Summary(id)
	status := value.Issue.Status
	state := idx.states[id]
	value.Issue = issueProjection(state.state, state.revision)
	value.Issue.Status = status
	return value
}

func (idx boardIssueIndex) references(ids []string) ([]issue.Reference, error) {
	values := make([]issue.Reference, 0, len(ids))
	for _, value := range ids {
		id := issue.ID(value)
		state, ok := idx.states[id]
		if !ok {
			return nil, fmt.Errorf("related issue %q is absent from board snapshot", id)
		}
		summary := idx.summary(id)
		status := summary.Issue.Status
		values = append(values, issue.Reference{
			ID:        id.String(),
			Title:     state.state.Title(),
			Type:      kindText(state.state.Kind()),
			Status:    status,
			Priority:  priorityNumber(state.state.Priority()),
			Conflicts: state.state.Conflicts(),
		})
	}
	idx.orderReferencesByCreation(values)
	return values, nil
}

func (idx boardIssueIndex) openReferences(ids []string) ([]issue.Reference, error) {
	values := make([]issue.Reference, 0, len(ids))
	for _, value := range ids {
		id := issue.ID(value)
		state, ok := idx.states[id]
		if !ok {
			return nil, fmt.Errorf("related issue %q is absent from board snapshot", id)
		}
		if lifecycle := state.state.Lifecycle(); lifecycle == nil || *lifecycle == issue.LifecycleOpen {
			summary := idx.summary(id)
			values = append(values, issue.Reference{
				ID:        id.String(),
				Title:     state.state.Title(),
				Type:      kindText(state.state.Kind()),
				Status:    summary.Issue.Status,
				Priority:  priorityNumber(state.state.Priority()),
				Conflicts: state.state.Conflicts(),
			})
		}
	}
	idx.orderReferencesByCreation(values)
	return values, nil
}

// orderReferencesByCreation applies user-visible issue order to references
// already validated against this board snapshot.
func (idx boardIssueIndex) orderReferencesByCreation(values []issue.Reference) {
	slices.SortFunc(values, func(left, right issue.Reference) int {
		return idx.compareIssueCreation(issue.ID(left.ID), issue.ID(right.ID))
	})
}

func (idx boardIssueIndex) compareIssueCreation(left, right issue.ID) int {
	if order := idx.states[left].state.Created().Compare(idx.states[right].state.Created()); order != 0 {
		return order
	}
	return cmp.Compare(left, right)
}

func (r *Repository) readIssueState(
	ctx context.Context,
	scope queryScope,
	id issue.ID,
) (issue.State, domainboard.Revision, error) {
	row, err := query.New(scope).BoardGetIssueState(
		ctx,
		query.BoardGetIssueStateParams{
			BoardID: r.boardID.String(),
			ID:      id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return issue.State{}, 0, errkind.Errorf(errkind.NotFound, "issue not found: %s", id)
	}
	if err != nil {
		return issue.State{}, 0, err
	}
	conflicts, err := r.readIssueConflictReferences(ctx, scope, row.Uid)
	if err != nil {
		return issue.State{}, 0, err
	}
	value, err := loadPersistedIssue(persistedIssueFromGet(row), conflicts[string(row.Uid)])
	return value.state, value.revision, err
}

func (r *Repository) readBoardIssueIndex(
	ctx context.Context,
	scope queryScope,
) (boardIssueIndex, error) {
	states, err := r.readBoardIssueStates(ctx, scope)
	if err != nil {
		return boardIssueIndex{}, err
	}
	presentation, err := r.readIssuePresentation(ctx, scope)
	if err != nil {
		return boardIssueIndex{}, err
	}
	return boardIssueIndex{states: states, presentation: presentation}, nil
}

func (r *Repository) readBoardIssueStates(
	ctx context.Context,
	scope queryScope,
) (map[issue.ID]issueStateAtRevision, error) {
	rows, err := query.New(scope).BoardListIssueStates(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	conflicts, err := r.readIssueConflictReferences(ctx, scope, nil)
	if err != nil {
		return nil, err
	}
	states := make(map[issue.ID]issueStateAtRevision)
	for _, row := range rows {
		value, err := loadPersistedIssue(persistedIssueFromList(row), conflicts[string(row.Uid)])
		if err != nil {
			return nil, err
		}
		states[value.state.ID()] = value
	}
	return states, nil
}

func persistedIssueFromGet(row query.BoardGetIssueStateRow) persistedIssue {
	return persistedIssue{
		uid:             append([]byte(nil), row.Uid...),
		id:              row.ID,
		title:           row.Title,
		kind:            row.Kind,
		lifecycle:       row.Lifecycle,
		priority:        row.Priority,
		createdAt:       row.CreatedAt,
		updatedAt:       row.UpdatedAt,
		closedAt:        row.ClosedAt,
		waitingReason:   row.WaitingReason,
		waitingSince:    row.WaitingSince,
		summary:         row.Summary,
		details:         row.Details,
		stateBody:       row.StateBody,
		stateNextAction: row.StateNextAction,
		stateAuthor:     row.StateAuthor,
		stateUpdatedAt:  row.StateUpdatedAt,
		stateSnapshotID: row.StateSnapshotLogEntryID,
		revision:        row.Revision,
		claimActor:      row.ClaimActor,
		claimStartedAt:  row.ClaimStartedAt,
		resultBody:      row.ResultBody,
	}
}

func persistedIssueFromList(row query.BoardListIssueStatesRow) persistedIssue {
	return persistedIssue{
		uid:             append([]byte(nil), row.Uid...),
		id:              row.ID,
		title:           row.Title,
		kind:            row.Kind,
		lifecycle:       row.Lifecycle,
		priority:        row.Priority,
		createdAt:       row.CreatedAt,
		updatedAt:       row.UpdatedAt,
		closedAt:        row.ClosedAt,
		waitingReason:   row.WaitingReason,
		waitingSince:    row.WaitingSince,
		summary:         row.Summary,
		details:         row.Details,
		stateBody:       row.StateBody,
		stateNextAction: row.StateNextAction,
		stateAuthor:     row.StateAuthor,
		stateUpdatedAt:  row.StateUpdatedAt,
		stateSnapshotID: row.StateSnapshotLogEntryID,
		revision:        row.Revision,
		claimActor:      row.ClaimActor,
		claimStartedAt:  row.ClaimStartedAt,
		resultBody:      row.ResultBody,
	}
}

func loadPersistedIssue(row persistedIssue, conflicts []issue.ConflictReference) (issueStateAtRevision, error) {
	uid, err := issue.ParseUID(row.uid)
	if err != nil {
		return issueStateAtRevision{}, err
	}
	var kind *issue.Kind
	if row.kind != nil {
		value, err := issue.NewKind(*row.kind)
		if err != nil {
			return issueStateAtRevision{}, err
		}
		kind = &value
	}
	var lifecycle *issue.Lifecycle
	if row.lifecycle != nil {
		value, err := issue.NewLifecycle(*row.lifecycle)
		if err != nil {
			return issueStateAtRevision{}, err
		}
		lifecycle = &value
	}
	var priority *issue.Priority
	if row.priority != nil {
		value, err := issue.NewPriority(int(*row.priority))
		if err != nil {
			return issueStateAtRevision{}, err
		}
		priority = &value
	}
	var claim *issue.ClaimState
	if (row.claimActor == nil) != (row.claimStartedAt == nil) {
		return issueStateAtRevision{}, errors.New("active claim is incomplete")
	}
	if row.claimActor != nil {
		claim = &issue.ClaimState{
			Actor:     issue.NewActor(*row.claimActor),
			StartedAt: *row.claimStartedAt,
		}
	}
	var waiting *issue.WaitingState
	if (row.waitingReason == nil) != (row.waitingSince == nil) {
		return issueStateAtRevision{}, errors.New("issue waiting state is incomplete")
	}
	if row.waitingReason != nil {
		waiting = &issue.WaitingState{
			Reason: *row.waitingReason,
			Since:  *row.waitingSince,
		}
	}
	recovery, err := loadRecoveryState(row)
	if err != nil {
		return issueStateAtRevision{}, err
	}
	state, err := issue.Load(issue.Snapshot{
		UID:           uid,
		ID:            issue.ID(row.id),
		Title:         row.title,
		Kind:          kind,
		Lifecycle:     lifecycle,
		Priority:      priority,
		ActiveClaim:   claim,
		Created:       row.createdAt,
		Updated:       row.updatedAt,
		ClosedAt:      row.closedAt,
		Waiting:       waiting,
		Summary:       stringValue(row.summary),
		Details:       stringValue(row.details),
		RecoveryState: recovery,
		Result:        stringValue(row.resultBody),
		Conflicts:     conflicts,
	})
	return issueStateAtRevision{
		state:    state,
		revision: domainboard.Revision(row.revision),
	}, err
}

func loadRecoveryState(row persistedIssue) (*issue.RecoveryState, error) {
	if row.stateBody == nil {
		if row.stateNextAction != nil || row.stateAuthor != nil ||
			row.stateUpdatedAt != nil || row.stateSnapshotID != nil {
			return nil, errors.New("issue State is incomplete")
		}
		return nil, nil
	}
	recovery := &issue.RecoveryState{
		Body:       *row.stateBody,
		NextAction: stringValue(row.stateNextAction),
		UpdatedAt:  row.stateUpdatedAt,
	}
	if row.stateAuthor != nil {
		recovery.Author = issue.NewActor(*row.stateAuthor)
	}
	if row.stateSnapshotID != nil {
		id, err := issue.NewLogID(*row.stateSnapshotID)
		if err != nil {
			return nil, err
		}
		recovery.SnapshotLogEntryID = &id
	}
	return recovery, nil
}

func issueProjection(state issue.State, revision domainboard.Revision) issue.Issue {
	var activeClaim *issue.ActiveClaim
	if claim := state.ActiveClaim(); claim != nil {
		activeClaim = &issue.ActiveClaim{
			Actor:     claim.Actor.String(),
			StartedAt: claim.StartedAt.Unix(),
		}
	}
	var waiting *issue.Waiting
	if state.Waiting() != nil {
		waiting = &issue.Waiting{
			Reason: state.Waiting().Reason,
			Since:  state.Waiting().Since.Unix(),
		}
	}
	recovery := state.RecoveryStateRecord()
	var nextAction *string
	if recovery != nil {
		nextAction = optionalString(recovery.NextAction)
	}
	return issue.Issue{
		ID:          state.ID().String(),
		Title:       state.Title(),
		Type:        kindText(state.Kind()),
		Lifecycle:   lifecycleText(state.Lifecycle()),
		Status:      state.Status().String(),
		Priority:    priorityNumber(state.Priority()),
		Assignee:    optionalString(state.Assignee().String()),
		ActiveClaim: activeClaim,
		Created:     state.Created().Unix(),
		Updated:     state.Updated().Unix(),
		StartedAt:   unixPointer(state.StartedAt()),
		Closed:      unixPointer(state.ClosedAt()),
		Waiting:     waiting,
		Summary:     optionalString(state.Summary()),
		Details:     optionalString(state.Details()),
		State:       optionalString(state.RecoveryState()),
		NextAction:  nextAction,
		Revision:    int64(revision),
		Conflicts:   state.Conflicts(),
	}
}

func waitingReason(waiting *issue.WaitingState) *string {
	if waiting == nil {
		return nil
	}
	value := waiting.Reason
	return &value
}

func waitingSince(waiting *issue.WaitingState) *time.Time {
	if waiting == nil {
		return nil
	}
	return &waiting.Since
}

func nullableText(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func unixPointer(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	unix := value.Unix()
	return &unix
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// readIssueConflictReferences loads compact absence metadata without reading
// alternative bodies. A nil UID selects all issues in the board snapshot.
func (r *Repository) readIssueConflictReferences(ctx context.Context, scope queryScope, uid []byte) (map[string][]issue.ConflictReference, error) {
	rows, err := query.New(scope).BoardReadIssueConflicts(ctx, query.BoardReadIssueConflictsParams{BoardID: r.boardID.String(), IssueUid: uid})
	if err != nil {
		return nil, err
	}
	values := make(map[string][]issue.ConflictReference)
	for _, row := range rows {
		field, err := issue.ParseField(row.Field)
		if err != nil {
			return nil, err
		}
		values[string(row.IssueUid)] = append(values[string(row.IssueUid)], issue.ConflictReference{ID: row.ID, Field: field, Key: row.MemberKey})
	}
	return values, nil
}

func kindText(value *issue.Kind) *string {
	if value == nil {
		return nil
	}
	return new(value.String())
}

func lifecycleText(value *issue.Lifecycle) *string {
	if value == nil {
		return nil
	}
	return new(value.String())
}

func priorityNumber(value *issue.Priority) *int {
	if value == nil {
		return nil
	}
	return new(value.Int())
}

func priorityInteger(value *issue.Priority) *int64 {
	if value == nil {
		return nil
	}
	return new(int64(value.Int()))
}
