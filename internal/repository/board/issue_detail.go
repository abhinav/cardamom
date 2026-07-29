package board

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ReadIssue reads one issue detail and optional inherited context from one snapshot.
func (r *Repository) ReadIssue(
	ctx context.Context,
	request issue.ReadRequest,
) (out issue.View, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	id, err := issue.NewID(request.IssueID)
	if err != nil {
		return out, err
	}
	index, err := r.readBoardIssueIndex(ctx, view)
	if err != nil {
		return out, err
	}
	out.Detail, err = r.readIssueDetail(ctx, view, index, id)
	if err != nil {
		return out, err
	}
	if request.ContextDepth != nil {
		out.Context, err = r.readIssueContext(ctx, view, index, id, *request.ContextDepth)
	}
	return out, err
}

func (r *Repository) readIssueDetail(
	ctx context.Context,
	scope queryScope,
	index boardIssueIndex,
	id issue.ID,
) (issue.Detail, error) {
	selected, ok := index.states[id]
	if !ok {
		return issue.Detail{}, errkind.Errorf(errkind.NotFound, "issue not found: %s", id)
	}
	summary := index.summary(id)
	dependsOnIDs, err := query.New(scope).BoardListPrerequisiteIDs(
		ctx,
		query.BoardListPrerequisiteIDsParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if err != nil {
		return issue.Detail{}, err
	}
	dependsOn, err := index.references(dependsOnIDs)
	if err != nil {
		return issue.Detail{}, err
	}
	blockIDs, err := query.New(scope).BoardListBlockIDs(
		ctx,
		query.BoardListBlockIDsParams{
			BoardID:        r.boardID.String(),
			PrerequisiteID: id.String(),
		},
	)
	if err != nil {
		return issue.Detail{}, err
	}
	blocks, err := index.references(blockIDs)
	if err != nil {
		return issue.Detail{}, err
	}
	logSummary, err := r.readLogSummary(ctx, scope, id)
	if err != nil {
		return issue.Detail{}, err
	}
	parent, err := r.readParent(ctx, scope, id)
	if err != nil {
		return issue.Detail{}, err
	}
	result, err := r.readOptionalResult(ctx, scope, id, selected.state.Title())
	if err != nil {
		return issue.Detail{}, err
	}
	decision, err := r.readCheckpointDecision(ctx, scope, id)
	if err != nil {
		return issue.Detail{}, err
	}
	story, err := r.readIssueStory(ctx, scope, index, id)
	if err != nil {
		return issue.Detail{}, err
	}
	return issue.Detail{
		Issue: summary.Issue, Labels: summary.Labels,
		State:     selected.state.RecoveryStateRecord(),
		DependsOn: dependsOn, Blocks: blocks, LogSummary: logSummary,
		ParentID: parent, CurrentResult: result, CheckpointDecision: decision,
		Story: story, Blocked: summary.Blocked,
	}, nil
}

func (r *Repository) readLogSummary(ctx context.Context, scope queryScope, id issue.ID) (issue.LogSummary, error) {
	row, err := query.New(scope).BoardGetIssueLogSummary(
		ctx,
		query.BoardGetIssueLogSummaryParams{
			ScopeBoardID:    r.boardID.String(),
			SelectedIssueID: id.String(),
		},
	)
	if err != nil {
		return issue.LogSummary{}, err
	}
	var latestID *issue.LogID
	if row.LatestLogID != "" {
		parsed, err := issue.NewLogID(row.LatestLogID)
		if err != nil {
			return issue.LogSummary{}, err
		}
		latestID = &parsed
	}
	return issue.LogSummary{Count: int(row.LogCount), LatestID: latestID}, nil
}

func (r *Repository) readCheckpointDecision(
	ctx context.Context,
	scope queryScope,
	id issue.ID,
) (*issue.CheckpointDecisionView, error) {
	row, err := query.New(scope).BoardGetCheckpointDecision(
		ctx,
		query.BoardGetCheckpointDecisionParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := issue.NewCheckpointOutcome(row.Outcome)
	if err != nil {
		return nil, err
	}
	return &issue.CheckpointDecisionView{
		Outcome:   parsed.String(),
		Reason:    row.Reason,
		DecidedAt: row.DecidedAt.Unix(),
		Revision:  row.Revision,
	}, nil
}

func (r *Repository) readParent(ctx context.Context, scope queryScope, id issue.ID) (*string, error) {
	parent, err := query.New(scope).BoardGetParentID(
		ctx,
		query.BoardGetParentIDParams{
			BoardID: r.boardID.String(),
			ChildID: id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &parent, nil
}

func (r *Repository) readOptionalResult(
	ctx context.Context,
	scope queryScope,
	id issue.ID,
	title string,
) (*issue.Result, error) {
	body, err := query.New(scope).BoardGetIssueResultBody(
		ctx,
		query.BoardGetIssueResultBodyParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &issue.Result{IssueID: id.String(), Title: title, Body: body}, nil
}

func (r *Repository) readIssueStory(
	ctx context.Context,
	scope queryScope,
	index boardIssueIndex,
	selected issue.ID,
) (issue.Story, error) {
	parents, err := r.readContainmentParents(ctx, scope)
	if err != nil {
		return issue.Story{}, err
	}
	if _, ok := index.states[selected]; !ok {
		return issue.Story{}, errkind.Errorf(errkind.NotFound, "issue not found: %s", selected)
	}

	included := map[issue.ID]struct{}{selected: {}}
	for current := selected; parents[current] != ""; current = parents[current] {
		parent := parents[current]
		included[parent] = struct{}{}
		for child, candidateParent := range parents {
			if candidateParent == parent {
				included[child] = struct{}{}
			}
		}
	}
	var addDescendants func(issue.ID)
	addDescendants = func(parent issue.ID) {
		for child, candidateParent := range parents {
			if candidateParent != parent {
				continue
			}
			included[child] = struct{}{}
			addDescendants(child)
		}
	}
	addDescendants(selected)

	ids := make([]issue.ID, 0, len(included))
	for id := range included {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	containment := make([]issue.ContainmentNode, 0, len(ids))
	for _, id := range ids {
		var parentID *string
		if parent := parents[id]; parent != "" && containsIssueID(included, parent) {
			value := parent.String()
			parentID = &value
		}
		containment = append(containment, issue.ContainmentNode{
			Reference: issue.Reference{
				ID:       id.String(),
				Title:    index.states[id].state.Title(),
				Type:     index.states[id].state.Kind().String(),
				Status:   index.summary(id).Issue.Status,
				Priority: index.states[id].state.Priority().Int(),
			},
			ParentID: parentID,
		})
	}
	dependsOnIDs, err := query.New(scope).BoardListPrerequisiteIDs(
		ctx,
		query.BoardListPrerequisiteIDsParams{
			BoardID: r.boardID.String(),
			IssueID: selected.String(),
		},
	)
	if err != nil {
		return issue.Story{}, err
	}
	dependsOn, err := index.openReferences(dependsOnIDs)
	if err != nil {
		return issue.Story{}, err
	}
	blockIDs, err := query.New(scope).BoardListBlockIDs(
		ctx,
		query.BoardListBlockIDsParams{
			BoardID:        r.boardID.String(),
			PrerequisiteID: selected.String(),
		},
	)
	if err != nil {
		return issue.Story{}, err
	}
	blocks, err := index.openReferences(blockIDs)
	if err != nil {
		return issue.Story{}, err
	}
	return issue.Story{Containment: containment, DependsOn: dependsOn, Blocks: blocks}, nil
}

func (r *Repository) readIssueContext(
	ctx context.Context,
	scope queryScope,
	index boardIssueIndex,
	id issue.ID,
	depth int,
) (*issue.Context, error) {
	description, err := query.New(scope).BoardGetIssueContextDescription(
		ctx,
		r.boardID.String(),
	)
	if err != nil {
		return nil, err
	}
	parents, err := r.readContainmentParents(ctx, scope)
	if err != nil {
		return nil, err
	}
	var ancestors []issue.ID
	for current := parents[id]; current != ""; current = parents[current] {
		ancestors = append(ancestors, current)
	}
	slices.Reverse(ancestors)
	if depth > 0 && len(ancestors) > depth {
		ancestors = ancestors[len(ancestors)-depth:]
	}
	entries := make([]issue.ContextEntry, 0, len(ancestors))
	for _, ancestor := range ancestors {
		summary, err := r.readLogSummary(ctx, scope, ancestor)
		if err != nil {
			return nil, err
		}
		state := index.states[ancestor]
		projection := issueProjection(state.state, state.revision)
		projection.Details = nil
		entries = append(entries, issue.ContextEntry{
			Issue: projection, LogSummary: summary,
			DetailsBytes: len(state.state.Details()),
		})
	}
	dependencies, err := query.New(scope).BoardListPrerequisiteIDs(
		ctx,
		query.BoardListPrerequisiteIDsParams{
			BoardID: r.boardID.String(),
			IssueID: id.String(),
		},
	)
	if err != nil {
		return nil, err
	}
	results := make([]issue.DependencyResult, 0)
	for _, dependency := range dependencies {
		dependencyID := issue.ID(dependency)
		references, err := index.references([]string{dependency})
		if err != nil {
			return nil, err
		}
		result, err := r.readOptionalResult(ctx, scope, dependencyID, references[0].Title)
		if err != nil {
			return nil, err
		}
		if result != nil {
			results = append(results, issue.DependencyResult{
				Issue: references[0], Body: result.Body,
			})
		}
	}
	return &issue.Context{
		Board:     issue.BoardDescription{Description: description},
		Ancestors: entries, DependencyResults: results,
	}, nil
}
