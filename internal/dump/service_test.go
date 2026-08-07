package dump

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestServiceSnapshotWholeBoardIsDeterministic(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	snapshot, err := service.snapshot(t.Context(), WholeBoard())
	require.NoError(t, err)

	assert.Equal(t, WholeBoard(), snapshot.Selection)
	assert.Equal(t, []string{"an-a", "an-b", "an-c", "an-d"}, issueIDs(snapshot.Issues))
	assert.Equal(t, []string{"alpha", "zeta"}, snapshot.Issues[0].Labels)
	assert.Equal(t, []Containment{
		{ChildID: "an-b", ParentID: "an-a"},
		{ChildID: "an-c", ParentID: "an-b"},
	}, snapshot.Containment)
	assert.Equal(t, []Dependency{
		{ChildID: "an-a", ParentID: "an-d"},
		{ChildID: "an-c", ParentID: "an-d"},
	}, snapshot.Dependencies)
	assert.Equal(t, []string{
		"cmt_00000000000000000000000000000009",
		"cmt_00000000000000000000000000000002",
	}, logEntryIDs(snapshot.LogEntries))
}

func TestServiceSnapshotOrdersPresentationByCreationBeforeIssueIdentity(t *testing.T) {
	service := newTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "issue-a", Title: "Newer", Created: 3},
			{ID: "issue-m", Title: "Parent", Created: 1},
			{ID: "issue-z", Title: "Older", Created: 2},
		},
		Containment: []Containment{
			{ChildID: "issue-a", ParentID: "issue-m"},
			{ChildID: "issue-z", ParentID: "issue-m"},
		},
		Dependencies: []Dependency{
			{ChildID: "issue-a", ParentID: "issue-m"},
			{ChildID: "issue-z", ParentID: "issue-m"},
		},
		Results: []Result{
			{IssueID: "issue-a", Body: "newer"},
			{IssueID: "issue-z", Body: "older"},
			{IssueID: "issue-m", Body: "parent"},
		},
	})

	snapshot, err := service.snapshot(t.Context(), WholeBoard())
	require.NoError(t, err)
	assert.Equal(t, []string{"issue-m", "issue-z", "issue-a"}, issueIDs(snapshot.Issues))
	assert.Equal(t, []Containment{
		{ChildID: "issue-z", ParentID: "issue-m"},
		{ChildID: "issue-a", ParentID: "issue-m"},
	}, snapshot.Containment)
	assert.Equal(t, []Dependency{
		{ChildID: "issue-z", ParentID: "issue-m"},
		{ChildID: "issue-a", ParentID: "issue-m"},
	}, snapshot.Dependencies)
	assert.Equal(t, []Result{
		{IssueID: "issue-m", Body: "parent"},
		{IssueID: "issue-z", Body: "older"},
		{IssueID: "issue-a", Body: "newer"},
	}, snapshot.Results)
}

func TestServiceSnapshotSelectedIssuesIncludeDescendants(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	snapshot, err := service.snapshot(t.Context(), SelectedIssues("an-b"))
	require.NoError(t, err)

	assert.Equal(t, SelectedIssues("an-b"), snapshot.Selection)
	assert.Equal(t, []string{"an-b", "an-c"}, issueIDs(snapshot.Issues))
	assert.Equal(t, []Containment{
		{ChildID: "an-b", ParentID: "an-a"},
		{ChildID: "an-c", ParentID: "an-b"},
	}, snapshot.Containment, "selection retains graph edges to issues outside the dump")
	assert.Equal(t, []Dependency{{ChildID: "an-c", ParentID: "an-d"}}, snapshot.Dependencies)
	assert.Equal(t, []Result{{IssueID: "an-c", Body: "complete"}}, snapshot.Results)
	assert.Equal(t, []string{"cmt_00000000000000000000000000000002"}, logEntryIDs(snapshot.LogEntries))
}

func TestServiceSnapshotSelectedIssuesIncludeEveryLifecycleState(t *testing.T) {
	service := newTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-root", Status: "ready"},
			{ID: "an-started", Status: "in_progress"},
			{ID: "an-closed", Status: "closed"},
			{ID: "an-cancelled", Status: "cancelled"},
		},
		Containment: []Containment{
			{ChildID: "an-started", ParentID: "an-root"},
			{ChildID: "an-closed", ParentID: "an-root"},
			{ChildID: "an-cancelled", ParentID: "an-root"},
		},
	})

	snapshot, err := service.snapshot(t.Context(), SelectedIssues("an-root"))
	require.NoError(t, err)
	assert.Equal(t, []string{"an-cancelled", "an-closed", "an-root", "an-started"}, issueIDs(snapshot.Issues))
}

func TestServiceSnapshotSelectedIssuesNormalizesOverlappingRoots(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	snapshot, err := service.snapshot(t.Context(), SelectedIssues("an-c", "an-a", "an-c"))
	require.NoError(t, err)

	assert.Equal(t, SelectedIssues("an-a"), snapshot.Selection)
	assert.Equal(t, []string{"an-a", "an-b", "an-c"}, issueIDs(snapshot.Issues))
	assert.Equal(t, []Containment{
		{ChildID: "an-b", ParentID: "an-a"},
		{ChildID: "an-c", ParentID: "an-b"},
	}, snapshot.Containment)
	assert.Equal(t, []Dependency{
		{ChildID: "an-a", ParentID: "an-d"},
		{ChildID: "an-c", ParentID: "an-d"},
	}, snapshot.Dependencies)
}

func TestServiceSnapshotNamedIssuesOnlyNormalizesIDs(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	snapshot, err := service.snapshot(t.Context(), NamedIssuesOnly("an-c", "an-a", "an-c"))
	require.NoError(t, err)

	assert.Equal(t, NamedIssuesOnly("an-a", "an-c"), snapshot.Selection)
	assert.Equal(t, []string{"an-a", "an-c"}, issueIDs(snapshot.Issues))
}

func TestServiceSnapshotRejectsUnknownIDs(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	_, err := service.snapshot(t.Context(), SelectedIssues("an-z", "an-missing", "an-z"))
	assert.Error(t, err)
	var unknown *UnknownIssuesError
	require.True(t, errors.As(err, &unknown))
	assert.Equal(t, []string{"an-missing", "an-z"}, unknown.IssueIDs)
	assert.EqualError(t, err, `unknown issue IDs: "an-missing", "an-z"`)
}

func TestServiceSnapshotRejectsUnspecifiedSelectionMode(t *testing.T) {
	service := newTestService(t, unorderedSnapshot())

	_, err := service.snapshot(t.Context(), Selection{})

	assert.EqualError(t, err, "unsupported dump selection mode 0")
}

func TestPublicationService_ExecuteRejectsUnknownIDsBeforePublication(t *testing.T) {
	publisher := &recordingPublisher{}
	service := newTestPublicationService(t, unorderedSnapshot(), publisher)

	_, err := service.Execute(t.Context(), Request{Selection: SelectedIssues("an-missing")})
	assert.EqualError(t, err, `unknown issue ID "an-missing"`)
	assert.Empty(t, publisher.publications)
}

func TestPublicationService_ExecuteRejectsSnapshotFromDifferentBoardBeforePublication(t *testing.T) {
	publisher := &recordingPublisher{}
	renderer, err := NewService(ServiceConfig{
		Reader:      staticSnapshotReader{snapshot: BoardSnapshot{BoardID: "board-other"}},
		Attachments: &attachmentSource{},
		Provenance:  testProvenance("board-1"),
	})
	require.NoError(t, err)
	service, err := NewPublicationService(renderer, publisher)
	require.NoError(t, err)

	_, err = service.Execute(t.Context(), Request{Selection: WholeBoard()})
	assert.EqualError(t, err, `dump snapshot board "board-other" does not match selected board "board-1"`)
	assert.Empty(t, publisher.publications)
}

func TestServiceSnapshotAllowsEmptyWholeBoard(t *testing.T) {
	service := newTestService(t, BoardSnapshot{BoardID: "board-empty", Revision: 1})

	snapshot, err := service.snapshot(t.Context(), WholeBoard())
	require.NoError(t, err)
	assert.Empty(t, snapshot.Issues)
	assert.NotNil(t, snapshot.Issues)
}

func TestServiceRenderOwnsSnapshotSelectionAndRendering(t *testing.T) {
	service, err := NewService(testServiceConfig(unorderedSnapshot()))
	require.NoError(t, err)

	result, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-c", "an-a", "an-c"),
	})
	require.NoError(t, err)
	assert.Equal(t, "board-1", result.Provenance.BoardID)
	assert.Equal(t, int64(12), result.Revision)
	assert.Equal(t, NamedIssuesOnly("an-a", "an-c"), result.Selection)
	assert.Equal(t, 2, result.IssueCount)
	assert.Equal(t, []string{
		"issues/an-a.md",
		"issues/an-c.md",
	}, renderedPaths(result.Files))
}

func TestServiceRenderRewritesIssueAndLogReferencesToCanonicalRelativePaths(t *testing.T) {
	const targetLogID = issue.LogID("log_0123456789abcdef0123456789abcdef")
	const absentLogID = issue.LogID("log_abcdef0123456789abcdef0123456789")
	summary := strings.Join([]string{
		"Issue: %an-target.",
		"Selected log: %" + targetLogID.String() + ".",
		"Unselected log: %" + absentLogID.String() + ".",
	}, "\n")
	service := newTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-source", Title: "Source", Summary: &summary},
			{ID: "an-target", Title: "Target"},
			{ID: "an-absent", Title: "Absent"},
		},
		LogEntries: []LogEntry{
			{
				ID: targetLogID, IssueID: "an-target",
				Author: new("captain"), Body: "Selected entry.",
				Created: new(int64(1)),
			},
			{
				ID: absentLogID, IssueID: "an-absent",
				Author: new("captain"), Body: "Absent entry.",
				Created: new(int64(2)),
			},
		},
	})

	result, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-source", "an-target"),
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"issues/an-source.md", "issues/an-target.md"}, renderedPaths(result.Files))
	assert.Contains(t, renderedBody(t, result.Files, "issues/an-source.md"), strings.Join([]string{
		"Issue: [%an-target](../issues/an-target.md).",
		"Selected log: [%log\\_" + strings.TrimPrefix(targetLogID.String(), "log_") +
			"](../issues/an-target.md#" + targetLogID.String() + ").",
		"Unselected log: %" + absentLogID.String() + ".",
	}, "\n"))
}

func TestServiceRenderRewritesBoardReferencesFromDumpRoot(t *testing.T) {
	const logID = issue.LogID("log_0123456789abcdef0123456789abcdef")
	description := "Issue: %an-target. Log: %" + logID.String() + "."
	service := newTestService(t, BoardSnapshot{
		BoardID: "board-1", Description: &description,
		Issues: []Issue{{ID: "an-target", Title: "Target"}},
		LogEntries: []LogEntry{{
			ID: logID, IssueID: "an-target", Author: new("captain"),
		}},
	})

	result, err := service.Render(t.Context(), RenderRequest{Selection: WholeBoard()})
	require.NoError(t, err)

	assert.Contains(t, renderedBody(t, result.Files, "README.md"),
		"Issue: [%an-target](issues/an-target.md). "+
			"Log: [%log\\_0123456789abcdef0123456789abcdef]"+
			"(issues/an-target.md#log_0123456789abcdef0123456789abcdef).")
}

func TestServiceRenderReferencesDoNotExpandSelection(t *testing.T) {
	const absentLogID = issue.LogID("log_abcdef0123456789abcdef0123456789")
	summary := strings.Join([]string{
		"Accepted: %an-target, bare an-target remains text.",
		"Unselected log: %" + absentLogID.String() + ".",
		`Escaped: \%an-escaped. Old: [[an-target]], [[an-target|label]], ![[an-target]].`,
		"Historical: %cmt_0123456789abcdef0123456789abcdef.",
		"Malformed: %log_0123456789abcdef0123456789abcdeg.",
		`[see %an-target](https://example.com)`,
		`![see %an-target](https://example.com/image.png)`,
		"`%an-code`",
		"```markdown\n%an-block\n```",
	}, "\n\n")
	service := newTestService(t, BoardSnapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-source", Title: "Source", Summary: &summary},
			{ID: "an-target", Title: "Target"},
			{ID: "an-absent", Title: "Absent"},
		},
		LogEntries: []LogEntry{{
			ID: absentLogID, IssueID: "an-absent", Author: new("captain"),
		}},
	})

	result, err := service.Render(t.Context(), RenderRequest{
		Selection: NamedIssuesOnly("an-source"),
	})
	require.NoError(t, err)

	assert.Equal(t, NamedIssuesOnly("an-source"), result.Selection)
	assert.Equal(t, []string{"issues/an-source.md"}, renderedPaths(result.Files))
	body := renderedBody(t, result.Files, "issues/an-source.md")
	assert.Contains(t, body,
		"Accepted: %an-target, bare an-target remains text.")
	assert.NotContains(t, body, "../issues/an-target.md")
	for _, authored := range []string{
		"%an-escaped",
		"[[an-target]]",
		"[[an-target|label]]",
		"![[an-target]]",
		"%" + absentLogID.String(),
		"%cmt_0123456789abcdef0123456789abcdef",
		"%log_0123456789abcdef0123456789abcdeg",
		"[see %an-target](https://example.com)",
		"![see %an-target](https://example.com/image.png)",
		"%an-code",
		"%an-block",
	} {
		assert.Contains(t, body, authored)
	}
}

func TestPublicationService_ExecuteOwnsRenderingAndPublication(t *testing.T) {
	publisher := &recordingPublisher{result: PublicationResult{
		Written: 2, Unchanged: 1, Removed: 1,
	}}
	service := newTestPublicationService(t, unorderedSnapshot(), publisher)

	result, err := service.Execute(t.Context(), Request{
		Destination: "dumps/cardamom",
		Selection:   NamedIssuesOnly("an-c", "an-a", "an-c"),
		Force:       ForceGenerated,
	})
	require.NoError(t, err)
	assert.Equal(t, ExecutionResult{
		Destination: "dumps/cardamom",
		BoardID:     "board-1",
		Revision:    12,
		Selection:   NamedIssuesOnly("an-a", "an-c"),
		Issues:      2,
		Written:     2,
		Unchanged:   1,
		Removed:     1,
	}, result)
	require.Len(t, publisher.publications, 1)
	publication := publisher.publications[0]
	assert.Equal(t, "dumps/cardamom", publication.Destination)
	assert.Equal(t, ForceGenerated, publication.Force)
	assert.Equal(t, NamedIssuesOnly("an-a", "an-c"), publication.Rendered.Selection)
	assert.Equal(t, []string{
		"issues/an-a.md",
		"issues/an-c.md",
	}, renderedPaths(publication.Rendered.Files))
}

func TestPublicationService_ExecuteStopsAtSnapshotFailure(t *testing.T) {
	publisher := &recordingPublisher{}
	readErr := errors.New("read failed")
	renderer, err := NewService(ServiceConfig{
		Reader:      errorSnapshotReader{err: readErr},
		Attachments: &attachmentSource{},
		Provenance:  testProvenance("board-1"),
	})
	require.NoError(t, err)
	service, err := NewPublicationService(renderer, publisher)
	require.NoError(t, err)

	_, err = service.Execute(t.Context(), Request{Selection: WholeBoard()})
	assert.EqualError(t, err, "read dump snapshot: read failed")
	assert.ErrorIs(t, err, readErr)
	assert.Empty(t, publisher.publications)
}

func TestPublicationService_ExecuteStopsAtRenderFailure(t *testing.T) {
	publisher := &recordingPublisher{}
	snapshot := BoardSnapshot{
		BoardID: "board-1", Issues: []Issue{{ID: "../escape", Title: "Escape"}},
	}
	service := newTestPublicationService(t, snapshot, publisher)

	_, err := service.Execute(t.Context(), Request{Selection: WholeBoard()})
	assert.EqualError(t, err, `render dump: render issue "../escape": issue ID is not safe for a dump path`)
	assert.Empty(t, publisher.publications)
}

func TestPublicationService_ExecutePropagatesPublicationFailure(t *testing.T) {
	publishErr := errors.New("disk failed")
	publisher := &recordingPublisher{err: publishErr}
	service := newTestPublicationService(t, unorderedSnapshot(), publisher)

	_, err := service.Execute(t.Context(), Request{Selection: WholeBoard()})
	assert.EqualError(t, err, "publish dump: disk failed")
	assert.ErrorIs(t, err, publishErr)
	assert.Len(t, publisher.publications, 1)
}

type staticSnapshotReader struct {
	snapshot BoardSnapshot
}

type errorSnapshotReader struct{ err error }

func (r errorSnapshotReader) ReadDumpSnapshot(context.Context) (BoardSnapshot, error) {
	return BoardSnapshot{}, r.err
}

type recordingPublisher struct {
	publications []Publication
	result       PublicationResult
	err          error
}

func (p *recordingPublisher) Publish(_ context.Context, publication Publication) (PublicationResult, error) {
	p.publications = append(p.publications, publication)
	return p.result, p.err
}

func (r staticSnapshotReader) ReadDumpSnapshot(context.Context) (BoardSnapshot, error) {
	return r.snapshot, nil
}

func newTestService(t *testing.T, snapshot BoardSnapshot) *Service {
	t.Helper()
	service, err := NewService(testServiceConfig(snapshot))
	require.NoError(t, err)
	return service
}

func newTestPublicationService(
	t *testing.T,
	snapshot BoardSnapshot,
	publisher Publisher,
) *PublicationService {
	t.Helper()
	service, err := NewPublicationService(newTestService(t, snapshot), publisher)
	require.NoError(t, err)
	return service
}

func testServiceConfig(snapshot BoardSnapshot) ServiceConfig {
	return ServiceConfig{
		Reader:      staticSnapshotReader{snapshot: snapshot},
		Attachments: &attachmentSource{},
		Provenance:  testProvenance(snapshot.BoardID),
	}
}

func testProvenance(boardID string) Provenance {
	return Provenance{
		ProjectID: "project-1", ProjectName: "Project one",
		BoardID: boardID, BoardName: "Board one",
	}
}

func unorderedSnapshot() BoardSnapshot {
	description := "Shared context."
	return BoardSnapshot{
		BoardID: "board-1", Revision: 12, Description: &description,
		Issues: []Issue{
			{ID: "an-c", Title: "Child", Labels: []string{"child"}},
			{ID: "an-a", Title: "Root", Labels: []string{"zeta", "alpha"}},
			{ID: "an-d", Title: "Dependency"},
			{ID: "an-b", Title: "Middle"},
		},
		Containment: []Containment{
			{ChildID: "an-c", ParentID: "an-b"},
			{ChildID: "an-b", ParentID: "an-a"},
		},
		Dependencies: []Dependency{
			{ChildID: "an-c", ParentID: "an-d"},
			{ChildID: "an-a", ParentID: "an-d"},
		},
		Results: []Result{{IssueID: "an-c", Body: "complete"}, {IssueID: "an-d", Body: "ready"}},
		LogEntries: []LogEntry{
			{ID: "cmt_00000000000000000000000000000009", IssueID: "an-a"},
			{ID: "cmt_00000000000000000000000000000002", IssueID: "an-c"},
		},
	}
}

func issueIDs(issues []Issue) []string {
	ids := make([]string, len(issues))
	for index, issue := range issues {
		ids[index] = issue.ID
	}
	return ids
}

func logEntryIDs(logEntries []LogEntry) []string {
	ids := make([]string, len(logEntries))
	for index, logEntry := range logEntries {
		ids[index] = logEntry.ID.String()
	}
	return ids
}
