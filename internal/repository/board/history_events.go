package board

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

func issueOriginEvent(state issue.State) boardhistory.Event {
	snapshot := state.Snapshot()
	return boardhistory.Event{
		OccurredAt: snapshot.Created,
		Payload: &boardhistory.IssueOrigin{
			UID:       snapshot.UID,
			PublicID:  snapshot.ID.String(),
			Title:     snapshot.Title,
			Kind:      snapshot.Kind.String(),
			Lifecycle: snapshot.Lifecycle.String(),
			Priority:  int64(snapshot.Priority.Int()),
			CreatedAt: snapshot.Created,
			UpdatedAt: snapshot.Updated,
			ClosedAt:  snapshot.ClosedAt,
			Summary:   historyMarkdownPointer(snapshot.Summary),
			Details:   historyMarkdownPointer(snapshot.Details),
			Waiting:   snapshot.Waiting,
		},
	}
}

func recordLabelChanges(
	mutation *mutation,
	uid issue.UID,
	before []string,
	after []issue.Label,
) {
	old := make(map[string]struct{}, len(before))
	for _, label := range before {
		old[label] = struct{}{}
	}
	current := make(map[string]struct{}, len(after))
	for _, label := range after {
		current[label.String()] = struct{}{}
	}
	for _, label := range slices.Sorted(maps.Keys(old)) {
		if _, ok := current[label]; !ok {
			mutation.record(boardhistory.Event{
				OccurredAt: mutation.occurredAt,
				Payload: &boardhistory.IssueLabel{
					IssueUID: uid, Label: label,
				},
			})
		}
	}
	for _, label := range slices.Sorted(maps.Keys(current)) {
		if _, ok := old[label]; !ok {
			mutation.record(boardhistory.Event{
				OccurredAt: mutation.occurredAt,
				Payload: &boardhistory.IssueLabel{
					IssueUID: uid, Label: label, Present: true,
				},
			})
		}
	}
}

func (r *Repository) recordDependencyChanges(
	ctx context.Context,
	mutation *mutation,
	issueUID issue.UID,
	before []string,
	after []issue.ID,
) error {
	old := make(map[issue.ID]struct{}, len(before))
	for _, id := range before {
		old[issue.ID(id)] = struct{}{}
	}
	current := make(map[issue.ID]struct{}, len(after))
	for _, id := range after {
		current[id] = struct{}{}
	}
	for _, id := range slices.Sorted(maps.Keys(old)) {
		if _, ok := current[id]; ok {
			continue
		}
		prerequisiteUID, err := r.readIssueUID(ctx, mutation.change, id)
		if err != nil {
			return err
		}
		mutation.record(boardhistory.Event{
			OccurredAt: mutation.occurredAt,
			Payload: &boardhistory.IssueDependency{
				IssueUID:        issueUID,
				PrerequisiteUID: prerequisiteUID,
			},
		})
	}
	for _, id := range slices.Sorted(maps.Keys(current)) {
		if _, ok := old[id]; ok {
			continue
		}
		prerequisiteUID, err := r.readIssueUID(ctx, mutation.change, id)
		if err != nil {
			return err
		}
		mutation.record(boardhistory.Event{
			OccurredAt: mutation.occurredAt,
			Payload: &boardhistory.IssueDependency{
				IssueUID:        issueUID,
				PrerequisiteUID: prerequisiteUID,
				Present:         true,
			},
		})
	}
	return nil
}

func (r *Repository) recordParentChange(
	ctx context.Context,
	mutation *mutation,
	childUID issue.UID,
	before *string,
	after *issue.ID,
) error {
	unchanged := before == nil && after == nil
	if before != nil && after != nil {
		unchanged = *before == after.String()
	}
	if unchanged {
		return nil
	}
	if before != nil {
		parentUID, err := r.readIssueUID(ctx, mutation.change, issue.ID(*before))
		if err != nil {
			return err
		}
		mutation.record(boardhistory.Event{
			OccurredAt: mutation.occurredAt,
			Payload: &boardhistory.IssueContainment{
				ChildUID:  childUID,
				ParentUID: parentUID,
			},
		})
	}
	if after != nil {
		parentUID, err := r.readIssueUID(ctx, mutation.change, *after)
		if err != nil {
			return err
		}
		mutation.record(boardhistory.Event{
			OccurredAt: mutation.occurredAt,
			Payload: &boardhistory.IssueContainment{
				ChildUID:  childUID,
				ParentUID: parentUID,
				Present:   true,
			},
		})
	}
	return nil
}

func recordIssueChanges(mutation *mutation, before, after issue.State) {
	at := mutation.occurredAt
	uid := after.UID()
	if before.ID() != after.ID() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssuePublicIDSet{
				IssueUID: uid, PublicID: after.ID().String(),
			},
		})
	}
	if before.Title() != after.Title() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueTitleSet{
				IssueUID: uid, Title: after.Title(),
			},
		})
	}
	if before.Kind() != after.Kind() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueKindSet{
				IssueUID: uid, Kind: after.Kind().String(),
			},
		})
	}
	if before.Lifecycle() != after.Lifecycle() ||
		!reflect.DeepEqual(before.ClosedAt(), after.ClosedAt()) {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueLifecycleSet{
				IssueUID:  uid,
				Lifecycle: after.Lifecycle().String(),
				ClosedAt:  after.ClosedAt(),
			},
		})
	}
	if before.Priority() != after.Priority() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssuePrioritySet{
				IssueUID: uid, Priority: int64(after.Priority().Int()),
			},
		})
	}
	if !reflect.DeepEqual(before.Waiting(), after.Waiting()) {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueWaitingSet{
				IssueUID: uid, Waiting: after.Waiting(),
			},
		})
	}
	if before.Summary() != after.Summary() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueSummarySet{
				IssueUID: uid, Summary: historyMarkdownPointer(after.Summary()),
			},
		})
	}
	if before.Details() != after.Details() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueDetailsSet{
				IssueUID: uid, Details: historyMarkdownPointer(after.Details()),
			},
		})
	}
	if !reflect.DeepEqual(before.RecoveryStateRecord(), after.RecoveryStateRecord()) {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload:    historyIssueState(after),
		})
	}
	if before.Result() != after.Result() {
		mutation.record(boardhistory.Event{
			OccurredAt: at,
			Payload: &boardhistory.IssueResult{
				IssueUID: uid,
				Body:     historyMarkdownPointer(after.Result()),
			},
		})
	}
}

func historyIssueState(state issue.State) *boardhistory.IssueState {
	value := &boardhistory.IssueState{
		IssueUID: state.UID(),
	}
	recovery := state.RecoveryStateRecord()
	if recovery == nil {
		return value
	}
	value.State = &boardhistory.StateRecord{
		Body:               recovery.Body,
		Author:             optionalHistoryActor(recovery.Author),
		UpdatedAt:          recovery.UpdatedAt,
		SnapshotLogEntryID: optionalHistoryLogID(recovery.SnapshotLogEntryID),
		NextAction:         historyTextPointer(recovery.NextAction),
	}
	return value
}

func historyMarkdownPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func historyTextPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalHistoryActor(value issue.Actor) *string {
	if value == "" {
		return nil
	}
	text := value.String()
	return &text
}

func optionalHistoryLogID(value *issue.LogID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}
