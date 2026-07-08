package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
)

// ReadDumpSnapshot materializes all dump state from one retained board snapshot.
func (r *Repository) ReadDumpSnapshot(ctx context.Context) (dump.BoardSnapshot, error) {
	snapshot, err := r.readBoardSnapshot(ctx)
	if err != nil {
		return dump.BoardSnapshot{}, err
	}
	out := dump.BoardSnapshot{
		BoardID: r.boardID.String(), Revision: snapshot.revision,
		Description:  snapshot.description,
		Issues:       make([]dump.Issue, 0, len(snapshot.issues)),
		Dependencies: make([]dump.Dependency, 0, len(snapshot.dependencies)),
		Containment:  make([]dump.Containment, 0, len(snapshot.containment)),
		Results:      make([]dump.Result, 0, len(snapshot.results)),
		LogEntries:   make([]dump.LogEntry, 0, len(snapshot.logEntries)),
	}
	for _, value := range snapshot.issues {
		projection := value.projection
		out.Issues = append(out.Issues, dump.Issue{
			ID: projection.ID, Title: projection.Title, Type: projection.Type,
			Status: projection.Status, Priority: projection.Priority,
			Assignee: projection.Assignee, Created: projection.Created,
			Updated: projection.Updated, StartedAt: projection.StartedAt,
			Closed:  projection.Closed,
			Summary: projection.Summary, Details: projection.Details,
			State: projection.State, NextAction: projection.NextAction,
			Revision: projection.Revision, Labels: value.labels,
		})
	}
	for _, value := range snapshot.dependencies {
		out.Dependencies = append(out.Dependencies, dump.Dependency{
			ChildID: value.childID, ParentID: value.parentID,
		})
	}
	for _, value := range snapshot.containment {
		out.Containment = append(out.Containment, dump.Containment{
			ChildID: value.childID, ParentID: value.parentID,
		})
	}
	for _, value := range snapshot.results {
		out.Results = append(out.Results, dump.Result{
			IssueID: value.issueID, Body: value.body,
		})
	}
	for _, value := range snapshot.logEntries {
		out.LogEntries = append(out.LogEntries, dump.LogEntry{
			ID: value.id, IssueID: value.issueID, Kind: value.kind,
			Author: value.author, Committer: value.committer,
			Body: value.body, NextAction: value.nextAction,
			Created: value.created,
		})
	}
	return out, nil
}

var (
	_ dump.SnapshotReader    = (*Repository)(nil)
	_ record.Changes         = (*Repository)(nil)
	_ issue.Reader           = (*Repository)(nil)
	_ issue.CompletionReader = (*Repository)(nil)
)
