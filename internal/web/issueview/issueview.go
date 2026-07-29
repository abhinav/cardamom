// Package issueview converts issue-domain records to browser protocol views.
package issueview

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Encoder converts issue-domain presentation records to generated messages.
type Encoder struct {
	markdown MarkdownRenderer
}

// New constructs an issue view encoder with the shared Markdown policy.
func New(markdown MarkdownRenderer) *Encoder {
	must.NotBeNilf(markdown, "issueview: Markdown renderer is required")
	return &Encoder{markdown: markdown}
}

// Summary converts one board-owned issue summary to its protocol view.
func (e *Encoder) Summary(
	boardID board.ID,
	value issue.Summary,
) (*privatev1.IssueSummary, error) {
	kind, err := issueType(value.Issue.Type)
	if err != nil {
		return nil, err
	}
	lifecycle, err := issueLifecycle(value.Issue.Lifecycle)
	if err != nil {
		return nil, err
	}
	status, err := issueStatus(value.Issue.Status, value.Blocked)
	if err != nil {
		return nil, err
	}
	var activeClaim *privatev1.ActiveClaim
	if value.Issue.ActiveClaim != nil {
		activeClaim = &privatev1.ActiveClaim{
			Actor:     value.Issue.ActiveClaim.Actor,
			StartedAt: timestamp(value.Issue.ActiveClaim.StartedAt),
		}
	}
	var waiting *privatev1.WaitingState
	if value.Issue.Waiting != nil {
		waiting = &privatev1.WaitingState{
			Reason: value.Issue.Waiting.Reason,
			Since:  timestamp(value.Issue.Waiting.Since),
		}
	}
	return &privatev1.IssueSummary{
		Id: value.Issue.ID, BoardId: boardID.String(), Title: value.Issue.Title,
		Type: kind, Lifecycle: lifecycle, Status: status,
		Priority: int32(value.Issue.Priority), ActiveClaim: activeClaim,
		CreatedAt: timestamp(value.Issue.Created), UpdatedAt: timestamp(value.Issue.Updated),
		StartedAt: optionalTimestamp(value.Issue.StartedAt),
		ClosedAt:  optionalTimestamp(value.Issue.Closed),
		Waiting:   waiting,
		Labels:    cloneStrings(value.Labels), Blocked: value.Blocked,
	}, nil
}

// Detail converts one issue and its current context and relationships.
func (e *Encoder) Detail(
	ctx context.Context,
	boardID board.ID,
	view issue.View,
) (*privatev1.IssueDetail, error) {
	batch := e.newMarkdownBatch(ctx, boardID)
	detail := view.Detail
	summary, err := e.Summary(boardID, issue.Summary{
		Issue: detail.Issue, Labels: detail.Labels, Blocked: detail.Blocked,
	})
	if err != nil {
		return nil, err
	}
	summaryContent := batch.addOptional(detail.Issue.Summary)
	details := batch.addOptional(detail.Issue.Details)
	state := batch.addOptional(detail.Issue.State)
	nextAction := batch.addOptional(detail.Issue.NextAction)
	var result *privatev1.MarkdownContent
	if detail.CurrentResult != nil {
		result = batch.add(detail.CurrentResult.Body)
	}
	contextValue, err := e.context(boardID, view.Context, batch)
	if err != nil {
		return nil, err
	}
	containment, err := containmentProjection(
		boardID,
		detail.Issue.ID,
		detail.Story.Containment,
	)
	if err != nil {
		return nil, err
	}
	prerequisites, err := e.References(boardID, detail.DependsOn)
	if err != nil {
		return nil, err
	}
	dependents, err := e.References(boardID, detail.Blocks)
	if err != nil {
		return nil, err
	}
	var latestLogID *string
	if detail.LogSummary.LatestID != nil {
		if _, err := issue.NewLogID(detail.LogSummary.LatestID.String()); err != nil {
			return nil, fmt.Errorf("convert latest log ID: %w", err)
		}
		value := detail.LogSummary.LatestID.String()
		latestLogID = &value
	}
	var decision *privatev1.CheckpointDecision
	if detail.CheckpointDecision != nil {
		outcome, err := checkpointOutcome(detail.CheckpointDecision.Outcome)
		if err != nil {
			return nil, err
		}
		decision = &privatev1.CheckpointDecision{
			Outcome:   outcome,
			Reason:    batch.add(detail.CheckpointDecision.Reason),
			DecidedAt: timestamp(detail.CheckpointDecision.DecidedAt),
			Revision:  detail.CheckpointDecision.Revision,
		}
	}
	converted := &privatev1.IssueDetail{
		Issue: summary, Summary: summaryContent, Details: details,
		State: state, NextAction: nextAction, Result: result,
		ExternalKeys: detail.Keys,
		Context:      contextValue, Containment: containment,
		Prerequisites: prerequisites, Dependents: dependents,
		LogCount:           uint32(detail.LogSummary.Count),
		LatestLogId:        latestLogID,
		CheckpointDecision: decision,
	}
	return converted, batch.render()
}

// Context converts optional board, ancestor, and dependency-result context.
func (e *Encoder) Context(
	ctx context.Context,
	boardID board.ID,
	value *issue.Context,
) (*privatev1.IssueContext, error) {
	batch := e.newMarkdownBatch(ctx, boardID)
	result, err := e.context(boardID, value, batch)
	if err != nil {
		return nil, err
	}
	return result, batch.render()
}

func (e *Encoder) context(
	boardID board.ID,
	value *issue.Context,
	batch *markdownBatch,
) (*privatev1.IssueContext, error) {
	result := &privatev1.IssueContext{
		Ancestors:         make([]*privatev1.AncestorContext, 0),
		DependencyResults: make([]*privatev1.DependencyResultContext, 0),
	}
	if value == nil {
		return result, nil
	}
	result.BoardDescription = batch.addOptional(value.Board.Description)
	for _, ancestor := range value.Ancestors {
		reference, err := issueDetailReference(
			boardID,
			issue.Detail{Issue: ancestor.Issue},
		)
		if err != nil {
			return nil, err
		}
		summary := batch.addOptional(ancestor.Issue.Summary)
		state := batch.addOptional(ancestor.Issue.State)
		nextAction := batch.addOptional(ancestor.Issue.NextAction)
		result.Ancestors = append(result.Ancestors, &privatev1.AncestorContext{
			Issue: reference, Summary: summary, State: state,
			NextAction:   nextAction,
			LogCount:     uint32(ancestor.LogSummary.Count),
			DetailsBytes: uint64(ancestor.DetailsBytes),
		})
	}
	for _, dependency := range value.DependencyResults {
		reference, err := issueReference(boardID, dependency.Issue)
		if err != nil {
			return nil, err
		}
		body := batch.add(dependency.Body)
		result.DependencyResults = append(
			result.DependencyResults,
			&privatev1.DependencyResultContext{Issue: reference, Result: body},
		)
	}
	return result, nil
}

// References converts issue relationship records in their supplied order.
func (e *Encoder) References(
	boardID board.ID,
	values []issue.Reference,
) ([]*privatev1.RelatedIssue, error) {
	result := make([]*privatev1.RelatedIssue, 0, len(values))
	for _, value := range values {
		converted, err := issueReference(boardID, value)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

// LogEntry converts one typed Log event and renders its Markdown body.
func (e *Encoder) LogEntry(
	ctx context.Context,
	boardID board.ID,
	value issue.LogEntry,
) (*privatev1.LogEntry, error) {
	batch := e.newMarkdownBatch(ctx, boardID)
	result, err := e.logEntry(value, batch)
	if err != nil {
		return nil, err
	}
	return result, batch.render()
}

// LogEntries converts typed Log events and renders their bodies in one batch.
func (e *Encoder) LogEntries(
	ctx context.Context,
	boardID board.ID,
	values []issue.LogEntry,
) ([]*privatev1.LogEntry, error) {
	batch := e.newMarkdownBatch(ctx, boardID)
	result := make([]*privatev1.LogEntry, 0, len(values))
	for _, value := range values {
		converted, err := e.logEntry(value, batch)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, batch.render()
}

func (e *Encoder) logEntry(
	value issue.LogEntry,
	batch *markdownBatch,
) (*privatev1.LogEntry, error) {
	if _, err := issue.NewLogID(value.ID.String()); err != nil {
		return nil, fmt.Errorf("convert log ID: %w", err)
	}
	body := batch.add(value.Body)
	nextAction := batch.addOptional(value.NextAction)
	entry := &privatev1.LogEntry{
		Id: value.ID.String(), IssueId: value.IssueID,
	}
	switch value.Kind {
	case issue.LogEntryKindPost.String():
		if value.Author == nil {
			return nil, errors.New("convert post author: required")
		}
		if value.Created == nil {
			return nil, errors.New("convert post creation time: required")
		}
		entry.Payload = &privatev1.LogEntry_Post{
			Post: &privatev1.LogPost{
				Actor:     *value.Author,
				Body:      body,
				CreatedAt: timestamp(*value.Created),
			},
		}
	case issue.LogEntryKindStateSnapshot.String():
		entry.Payload = &privatev1.LogEntry_StateSnapshot{
			StateSnapshot: &privatev1.StateSnapshot{
				Body:       body,
				NextAction: nextAction,
				Author:     value.Author,
				Committer:  value.Committer,
				CreatedAt:  optionalTimestamp(value.Created),
			},
		}
	default:
		return nil, fmt.Errorf("convert log entry kind %q", value.Kind)
	}
	return entry, nil
}

// StateRecord converts one attributed mutable State and renders its Markdown.
func (e *Encoder) StateRecord(
	ctx context.Context,
	boardID board.ID,
	value *issue.RecoveryState,
) (*privatev1.StateRecord, error) {
	if value == nil {
		return nil, nil
	}
	body, err := e.Markdown(ctx, boardID, value.Body)
	if err != nil {
		return nil, err
	}
	var snapshotID *string
	if value.SnapshotLogEntryID != nil {
		snapshotID = new(value.SnapshotLogEntryID.String())
	}
	var nextAction *privatev1.MarkdownContent
	if value.NextAction != "" {
		nextAction, err = e.Markdown(ctx, boardID, value.NextAction)
		if err != nil {
			return nil, err
		}
	}
	return &privatev1.StateRecord{
		Body:               body,
		NextAction:         nextAction,
		Actor:              optionalActor(value.Author),
		UpdatedAt:          optionalTime(value.UpdatedAt),
		SnapshotLogEntryId: snapshotID,
	}, nil
}

func optionalActor(actor issue.Actor) *string {
	if actor == "" {
		return nil
	}
	return new(actor.String())
}

func optionalTime(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}

// PresentationStatus returns the status presented for an issue summary.
func PresentationStatus(value issue.Issue, blocked bool) string {
	if value.Status == "ready" && blocked {
		return "blocked"
	}
	return value.Status
}

func issueType(value string) (privatev1.IssueType, error) {
	switch value {
	case "workstream":
		return privatev1.IssueType_ISSUE_TYPE_WORKSTREAM, nil
	case "task":
		return privatev1.IssueType_ISSUE_TYPE_TASK, nil
	case "checkpoint":
		return privatev1.IssueType_ISSUE_TYPE_CHECKPOINT, nil
	case "routine":
		return privatev1.IssueType_ISSUE_TYPE_ROUTINE, nil
	default:
		return 0, fmt.Errorf("convert issue type %q", value)
	}
}

func issueLifecycle(value string) (privatev1.IssueLifecycle, error) {
	switch value {
	case "open":
		return privatev1.IssueLifecycle_ISSUE_LIFECYCLE_OPEN, nil
	case "closed":
		return privatev1.IssueLifecycle_ISSUE_LIFECYCLE_CLOSED, nil
	case "cancelled":
		return privatev1.IssueLifecycle_ISSUE_LIFECYCLE_CANCELLED, nil
	default:
		return 0, fmt.Errorf("convert issue lifecycle %q", value)
	}
}

func issueStatus(value string, blocked bool) (privatev1.IssueStatus, error) {
	if value == "ready" && blocked {
		return privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, nil
	}
	switch value {
	case "ready":
		return privatev1.IssueStatus_ISSUE_STATUS_READY, nil
	case "blocked":
		return privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, nil
	case "in_progress":
		return privatev1.IssueStatus_ISSUE_STATUS_IN_PROGRESS, nil
	case "waiting":
		return privatev1.IssueStatus_ISSUE_STATUS_WAITING, nil
	case "closed":
		return privatev1.IssueStatus_ISSUE_STATUS_CLOSED, nil
	case "cancelled":
		return privatev1.IssueStatus_ISSUE_STATUS_CANCELLED, nil
	default:
		return 0, fmt.Errorf("convert issue status %q", value)
	}
}

func checkpointOutcome(value string) (privatev1.CheckpointOutcome, error) {
	switch value {
	case "approved":
		return privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_APPROVED, nil
	case "denied":
		return privatev1.CheckpointOutcome_CHECKPOINT_OUTCOME_DENIED, nil
	default:
		return 0, fmt.Errorf("convert checkpoint outcome %q", value)
	}
}

func issueReference(
	boardID board.ID,
	value issue.Reference,
) (*privatev1.RelatedIssue, error) {
	kind, err := issueType(value.Type)
	if err != nil {
		return nil, err
	}
	status, err := issueStatus(value.Status, false)
	if err != nil {
		return nil, err
	}
	return &privatev1.RelatedIssue{
		Id: value.ID, BoardId: boardID.String(), Title: value.Title,
		Type: kind, Status: status, Priority: int32(value.Priority),
	}, nil
}

func issueDetailReference(
	boardID board.ID,
	value issue.Detail,
) (*privatev1.RelatedIssue, error) {
	return issueReference(boardID, issue.Reference{
		ID: value.Issue.ID, Title: value.Issue.Title, Type: value.Issue.Type,
		Status:   PresentationStatus(value.Issue, value.Blocked),
		Priority: value.Issue.Priority,
	})
}

// containmentProjection turns the domain's parent-linked neighborhood into a
// stable preorder and marks the selected issue's ancestor path.
func containmentProjection(
	boardID board.ID,
	selectedID string,
	values []issue.ContainmentNode,
) (*privatev1.ContainmentProjection, error) {
	result := &privatev1.ContainmentProjection{Nodes: make([]*privatev1.HierarchyNode, 0, len(values))}
	if len(values) == 0 {
		return result, nil
	}
	byID := make(map[string]issue.ContainmentNode, len(values))
	children := make(map[string][]string)
	var roots []string
	for _, value := range values {
		if _, duplicate := byID[value.ID]; duplicate {
			return nil, fmt.Errorf("duplicate containment issue %q", value.ID)
		}
		byID[value.ID] = value
	}
	if _, ok := byID[selectedID]; !ok {
		return nil, fmt.Errorf("selected issue %q is absent from containment projection", selectedID)
	}
	for _, value := range values {
		if value.ParentID == nil {
			roots = append(roots, value.ID)
			continue
		}
		if _, ok := byID[*value.ParentID]; !ok {
			roots = append(roots, value.ID)
			continue
		}
		children[*value.ParentID] = append(children[*value.ParentID], value.ID)
	}
	selectedPath := make(map[string]struct{})
	for current := selectedID; current != ""; {
		if _, duplicate := selectedPath[current]; duplicate {
			return nil, fmt.Errorf("containment cycle at %q", current)
		}
		selectedPath[current] = struct{}{}
		value, ok := byID[current]
		if !ok || value.ParentID == nil {
			break
		}
		current = *value.ParentID
	}
	visited := make(map[string]struct{}, len(values))
	var appendTree func(string, uint32) error
	appendTree = func(id string, depth uint32) error {
		if _, duplicate := visited[id]; duplicate {
			return fmt.Errorf("containment cycle at %q", id)
		}
		visited[id] = struct{}{}
		value := byID[id]
		reference, err := issueReference(boardID, value.Reference)
		if err != nil {
			return err
		}
		_, onSelectedPath := selectedPath[id]
		result.Nodes = append(result.Nodes, &privatev1.HierarchyNode{
			Issue: reference, ParentId: cloneString(value.ParentID),
			Depth: depth, SelectedPath: onSelectedPath,
		})
		for _, child := range children[id] {
			if err := appendTree(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := appendTree(root, 0); err != nil {
			return nil, err
		}
	}
	if len(visited) != len(values) {
		return nil, errors.New("containment projection is cyclic")
	}
	return result, nil
}

func timestamp(seconds int64) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(seconds, 0).UTC())
}

func optionalTimestamp(seconds *int64) *timestamppb.Timestamp {
	if seconds == nil {
		return nil
	}
	return timestamp(*seconds)
}

func cloneStrings(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
