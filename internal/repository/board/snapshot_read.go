package board

import (
	"context"
	"errors"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// boardSnapshot is the repository's private projection for dump reads.
// Every field is materialized through one retained store view.
type boardSnapshot struct {
	revision     int64
	description  *string
	issues       []snapshotIssue
	dependencies []snapshotDependency
	containment  []snapshotContainment
	results      []snapshotResult
	logEntries   []snapshotLogEntry
}

type snapshotIssue struct {
	projection issue.Issue
	labels     []string
}

type snapshotDependency struct {
	childID  string
	parentID string
}

type snapshotContainment struct {
	childID  string
	parentID string
}

type snapshotResult struct {
	issueID string
	body    string
}

type snapshotLogEntry struct {
	id         issue.LogID
	issueID    string
	kind       string
	author     *string
	committer  *string
	body       string
	nextAction *string
	created    *int64
}

// readBoardSnapshot materializes the complete shared board projection before
// releasing its read snapshot.
func (r *Repository) readBoardSnapshot(ctx context.Context) (out boardSnapshot, err error) {
	view, err := r.store.View(ctx)
	if err != nil {
		return out, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	out.revision, err = view.CanonicalRevision(ctx)
	if err != nil {
		return out, err
	}
	description, err := query.New(view).BoardGetSnapshotDescription(
		ctx,
		r.boardID.String(),
	)
	if err != nil {
		return out, err
	}
	out.description = description

	ids, err := r.listBoardIssueIDs(ctx, view)
	if err != nil {
		return out, err
	}
	index, err := r.readBoardIssueIndex(ctx, view)
	if err != nil {
		return out, err
	}
	out.issues = make([]snapshotIssue, 0, len(ids))
	for _, id := range ids {
		summary := index.summary(id)
		out.issues = append(out.issues, snapshotIssue{
			projection: summary.Issue,
			labels:     summary.Labels,
		})
	}

	out.dependencies, err = r.readSnapshotDependencies(ctx, view)
	if err != nil {
		return out, err
	}
	out.containment, err = r.readSnapshotContainment(ctx, view)
	if err != nil {
		return out, err
	}
	out.results, err = r.readSnapshotResults(ctx, view)
	if err != nil {
		return out, err
	}
	out.logEntries, err = r.readSnapshotLogEntries(ctx, view)
	return out, err
}

func (r *Repository) readSnapshotDependencies(
	ctx context.Context,
	scope queryScope,
) (out []snapshotDependency, err error) {
	rows, err := query.New(scope).BoardListSnapshotDependencies(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make([]snapshotDependency, 0, len(rows))
	for _, row := range rows {
		values = append(values, snapshotDependency{
			childID:  row.IssueID,
			parentID: row.PrerequisiteID,
		})
	}
	return values, nil
}

func (r *Repository) readSnapshotContainment(
	ctx context.Context,
	scope queryScope,
) (out []snapshotContainment, err error) {
	rows, err := query.New(scope).BoardListSnapshotContainment(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make([]snapshotContainment, 0, len(rows))
	for _, row := range rows {
		values = append(values, snapshotContainment{
			childID:  row.ChildID,
			parentID: row.ParentID,
		})
	}
	return values, nil
}

func (r *Repository) readSnapshotResults(
	ctx context.Context,
	scope queryScope,
) (out []snapshotResult, err error) {
	rows, err := query.New(scope).BoardListSnapshotResults(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make([]snapshotResult, 0, len(rows))
	for _, row := range rows {
		values = append(values, snapshotResult{issueID: row.IssueID, body: row.Body})
	}
	return values, nil
}

func (r *Repository) readSnapshotLogEntries(
	ctx context.Context,
	scope queryScope,
) (out []snapshotLogEntry, err error) {
	rows, err := query.New(scope).BoardListSnapshotLogEntries(ctx, r.boardID.String())
	if err != nil {
		return nil, err
	}
	values := make([]snapshotLogEntry, 0, len(rows))
	for _, row := range rows {
		id, err := issue.NewLogID(row.ID)
		if err != nil {
			return nil, err
		}
		values = append(values, snapshotLogEntry{
			id: id, issueID: row.IssueID, kind: row.Kind,
			author: row.Author, committer: row.Committer,
			body: row.Body, nextAction: row.NextAction,
			created: optionalUnixTime(row.CreatedAt),
		})
	}
	return values, nil
}
