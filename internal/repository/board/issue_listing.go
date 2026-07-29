package board

import (
	"context"
	"errors"
	"slices"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ListIssues reads filtered issue summaries from one coherent board snapshot.
func (r *Repository) ListIssues(
	ctx context.Context,
	request issue.ListRequest,
) (out []issue.Summary, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return r.listIssues(ctx, view, request)
}

// ListIssuesSnapshot reads issue summaries and the canonical board revision
// from one coherent repository view.
func (r *Repository) ListIssuesSnapshot(
	ctx context.Context,
	request issue.ListRequest,
) (out issue.ListSnapshot, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	out.Issues, out.Total, err = r.listIssuesWithTotal(ctx, view, request)
	if err != nil {
		return out, err
	}
	if err := r.readChangeCursor(ctx, view, &out.Cursor); err != nil {
		return out, err
	}
	return out, nil
}

func (r *Repository) listIssues(
	ctx context.Context,
	scope queryScope,
	request issue.ListRequest,
) ([]issue.Summary, error) {
	issues, _, err := r.listIssuesWithTotal(ctx, scope, request)
	return issues, err
}

func (r *Repository) listIssuesWithTotal(
	ctx context.Context,
	scope queryScope,
	request issue.ListRequest,
) ([]issue.Summary, int, error) {
	index, err := r.readBoardIssueIndex(ctx, scope)
	if err != nil {
		return nil, 0, err
	}
	descendants, err := r.descendantSet(ctx, scope, request.UnderID)
	if err != nil {
		return nil, 0, err
	}
	issues, total := r.filterIssueSummaries(index, request, descendants)
	return issues, total, nil
}

func (r *Repository) filterIssueSummaries(
	index boardIssueIndex,
	request issue.ListRequest,
	descendants map[issue.ID]struct{},
) ([]issue.Summary, int) {
	values := make([]issue.BoardIssueSummary, 0, len(index.states))
	for id := range index.states {
		summary := index.summary(id)
		if !matchesIssueRequest(request, summary, descendants) {
			continue
		}
		values = append(values, issue.BoardIssueSummary{
			BoardID: r.boardID.String(),
			Summary: summary,
		})
	}
	total := len(values)
	values = issue.OrderSummaries(request, values)
	out := make([]issue.Summary, len(values))
	for index, value := range values {
		out[index] = value.Summary
	}
	return out, total
}

// ListReadyIssues reads executable work without unresolved prerequisites.
func (r *Repository) ListReadyIssues(ctx context.Context, request issue.ListReadyRequest) (out []issue.Summary, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	values, err := r.listIssues(ctx, view, issue.ListRequest{})
	if err != nil {
		return nil, err
	}
	ready := make([]issue.Summary, 0)
	for _, value := range values {
		eligibility, err := execution.EvaluateEligibility(value)
		if err != nil {
			return nil, err
		}
		if eligibility.ReadyForClaim() {
			ready = append(ready, value)
		}
	}
	if request.Limit > 0 && len(ready) > request.Limit {
		ready = ready[:request.Limit]
	}
	return ready, nil
}

// ListBlockedIssues reads open non-routine issues with unresolved prerequisites.
func (r *Repository) ListBlockedIssues(ctx context.Context, request issue.ListBlockedRequest) (out []issue.Summary, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	values, err := r.listIssues(ctx, view, issue.ListRequest{})
	if err != nil {
		return nil, err
	}
	blocked := make([]issue.Summary, 0)
	for _, value := range values {
		eligibility, err := execution.EvaluateEligibility(value)
		if err != nil {
			return nil, err
		}
		if eligibility.Blocked() {
			blocked = append(blocked, value)
		}
	}
	if request.Limit > 0 && len(blocked) > request.Limit {
		blocked = blocked[:request.Limit]
	}
	return blocked, nil
}

func matchesIssueRequest(
	request issue.ListRequest,
	summary issue.Summary,
	descendants map[issue.ID]struct{},
) bool {
	value := summary.Issue
	if descendants != nil {
		if _, ok := descendants[issue.ID(value.ID)]; !ok {
			return false
		}
	}
	if len(request.Statuses) > 0 {
		if !slices.Contains(request.Statuses, value.Status) {
			return false
		}
	}
	if len(request.Lifecycles) > 0 && !slices.Contains(request.Lifecycles, value.Lifecycle) {
		return false
	}
	if request.Assignee != nil && (value.Assignee == nil || *value.Assignee != *request.Assignee) {
		return false
	}
	if request.NoAssignee && value.Assignee != nil {
		return false
	}
	if request.Type != "" && value.Type != request.Type {
		return false
	}
	if len(request.Types) > 0 && !slices.Contains(request.Types, value.Type) {
		return false
	}
	for _, label := range request.LabelsAll {
		if !slices.Contains(summary.Labels, label) {
			return false
		}
	}
	if len(request.LabelsAny) > 0 {
		matched := false
		for _, label := range request.LabelsAny {
			matched = matched || slices.Contains(summary.Labels, label)
		}
		if !matched {
			return false
		}
	}
	for _, label := range request.LabelsNone {
		if slices.Contains(summary.Labels, label) {
			return false
		}
	}
	if request.TitleContains != "" && !normalizedContains(value.Title, request.TitleContains) {
		return false
	}
	if request.TitleRegexp != nil && !request.TitleRegexp.MatchString(value.Title) {
		return false
	}
	return true
}

func normalizedContains(value, substring string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(substring))
}

func (r *Repository) readBlockedIssueIDs(
	ctx context.Context,
	scope queryScope,
) (out map[issue.ID]struct{}, err error) {
	ids, err := query.New(scope).BoardListBlockedIssueIDs(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	blocked := make(map[issue.ID]struct{})
	for _, id := range ids {
		blocked[issue.ID(id)] = struct{}{}
	}
	return blocked, nil
}

func containsIssueID(values map[issue.ID]struct{}, id issue.ID) bool {
	_, ok := values[id]
	return ok
}
