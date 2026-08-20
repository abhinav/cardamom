package dump

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestRenderProducesDeterministicLinkedMarkdown(t *testing.T) {
	description := "Board **context**."
	assignee := "alice|ops"
	started := int64(1_752_144_300)
	closed := int64(1_752_148_200)
	parentID := "an-parent"
	snapshot := Snapshot{
		BoardID: "board-1", Revision: 42, Description: &description,
		Issues: []Issue{
			{ID: "an-b", Title: "Child [beta]", Type: "task", Status: "closed", Priority: 2, Created: 1_752_144_000, Updated: 1_752_148_200, StartedAt: &started, Closed: &closed, Revision: 41},
			{ID: "an-a", Title: "Root | alpha", Type: "workstream", Status: "ready", Priority: 1, Assignee: &assignee, Created: 1_752_140_400, Updated: 1_752_144_000, Summary: new("Authored **Markdown**.\n\n- stays a list"), Details: new("Expanded material."), State: new("Run `card show`."), NextAction: new("Inspect the output."), Revision: 40, Labels: []string{"zeta", "a`b"}},
		},
		Containment:  []Containment{{ChildID: "an-a", ParentID: parentID}, {ChildID: "an-b", ParentID: "an-a"}},
		Dependencies: []Dependency{{ChildID: "an-a", ParentID: "an-dep"}, {ChildID: "an-blocked", ParentID: "an-a"}},
		Results:      []Result{{IssueID: "an-a", Body: "Result *body*."}},
		LogEntries: []LogEntry{{
			ID:      "cmt_00000000000000000000000000000007",
			IssueID: "an-a", Kind: "post",
			Author: new("reviewer_[1]"), Committer: new("reviewer_[1]"),
			Body: "LogEntry **body**.", Created: new(int64(1_752_144_600)),
		}},
		Provenance: testProvenance("board-1"),
		Selection:  SelectedIssues("an-a"),
	}

	rendered, err := render(snapshot)
	require.NoError(t, err)
	assert.Equal(t, testProvenance("board-1"), rendered.Provenance)
	assert.Equal(t, int64(42), rendered.Revision)
	assert.Equal(t, SelectedIssues("an-a"), rendered.Selection)
	assert.Equal(t, 2, rendered.IssueCount)
	assert.Equal(t, []string{
		"issues/an-a.md",
		"issues/an-b.md",
	}, renderedPaths(rendered.Files))
	assert.Equal(t, []string{"issue:an-a", "issue:an-b"}, renderedIdentities(rendered.Files))

	issue := renderedBody(t, rendered.Files, "issues/an-a.md")
	assert.NotContains(t, issue, "[Board dump]")
	assert.Contains(t, issue, "# an-a: Root \\| alpha")
	assert.Contains(t, issue, "| Labels | `` a`b ``, `zeta` |")
	assert.Contains(t, issue, "## Summary\n\nAuthored **Markdown**.\n\n- stays a list\n\n## Details\n\nExpanded material.\n\n## Current state")
	assert.Contains(t, issue, "**Next action**\n\nInspect the output.")
	assert.Contains(t, issue, "### Containment parent\n\n- `an-parent` (not included in this dump)")
	assert.Contains(t, issue, "### Containment children\n\n- [an-b: Child \\[beta\\]](../issues/an-b.md)")
	assert.Contains(t, issue, "## Descendant lifecycle summary\n\n| Status | Issues |\n| --- | ---: |\n| Ready | 0 |\n| Blocked | 0 |\n| In progress | 0 |\n| Waiting | 0 |\n| Closed | 1 |\n| Cancelled | 0 |")
	assert.Contains(t, issue, "## Descendant containment tree\n\n- [an-b: Child \\[beta\\]](../issues/an-b.md)")
	assert.Contains(t, issue, "### Depends on\n\n- `an-dep` (not included in this dump)")
	assert.Contains(t, issue, "### Blocks\n\n- `an-blocked` (not included in this dump)")
	assert.Contains(t, issue, "### reviewer\\_\\[1\\] at 2025-07-10T10:50:00Z (log entry cmt_00000000000000000000000000000007)\n\nLogEntry **body**.")
	assert.NotContains(t, issue, `<a id="cmt_00000000000000000000000000000007"></a>`)

	for _, file := range rendered.Files {
		content := generatedFileContent(t, file)
		assert.NotContains(t, string(content), "\r", file.Path())
		assert.True(t, len(content) > 0 && content[len(content)-1] == '\n', file.Path())
		assert.False(t, len(content) > 1 && content[len(content)-2] == '\n', file.Path())
	}

	shuffled := snapshot
	shuffled.Issues = slices.Clone(snapshot.Issues)
	slices.Reverse(shuffled.Issues)
	shuffled.Containment = slices.Clone(snapshot.Containment)
	slices.Reverse(shuffled.Containment)
	shuffled.Dependencies = slices.Clone(snapshot.Dependencies)
	slices.Reverse(shuffled.Dependencies)

	again, err := render(shuffled)
	require.NoError(t, err)
	assert.Equal(t, rendered.Provenance, again.Provenance)
	assert.Equal(t, rendered.Revision, again.Revision)
	assert.Equal(t, rendered.Selection, again.Selection)
	assert.Equal(t, rendered.IssueCount, again.IssueCount)
	assert.Equal(t, renderedPaths(rendered.Files), renderedPaths(again.Files))
	assert.Equal(t, renderedIdentities(rendered.Files), renderedIdentities(again.Files))
	assert.Equal(t, renderedContents(t, rendered.Files), renderedContents(t, again.Files))
}

func TestRenderEmptyBoardUsesExplicitAbsenceText(t *testing.T) {
	rendered, err := render(Snapshot{
		BoardID: "empty-board", Revision: 0,
		Provenance: testProvenance("empty-board"),
		Selection:  WholeBoard(),
	})
	require.NoError(t, err)
	require.Len(t, rendered.Files, 1)
	assert.Equal(t, "README.md", rendered.Files[0].Path())
	assert.Equal(t, "board", rendered.Files[0].Identity())
	content := generatedFileContent(t, rendered.Files[0])
	assert.Contains(t, string(content), "No board description.")
	assert.Contains(t, string(content), "No issues.")
}

func TestRenderWholeBoardUsesCanonicalPathsAndREADME(t *testing.T) {
	rendered, err := render(Snapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-w", Title: "Workstream", Type: "workstream", Status: "cancelled"},
			{ID: "an-t", Title: "Task", Type: "task", Status: "closed"},
		},
		Provenance: testProvenance("board-1"),
		Selection:  WholeBoard(),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"README.md", "issues/an-t.md", "issues/an-w.md"}, renderedPaths(rendered.Files))
	assert.Equal(t, []string{"board", "issue:an-t", "issue:an-w"}, renderedIdentities(rendered.Files))
	assert.Contains(t, renderedBody(t, rendered.Files, "README.md"), "[an-w](issues/an-w.md)")
	assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-w.md"), "[Board dump](../README.md)")
}

func TestRenderOrdersIssuePresentationByCreation(t *testing.T) {
	rendered, err := render(Snapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "issue-a", Title: "Newer", Type: "task", Status: "ready", Priority: 0, Created: 3},
			{ID: "issue-m", Title: "Parent", Type: "workstream", Status: "ready", Priority: 2, Created: 1},
			{ID: "issue-z", Title: "Older", Type: "task", Status: "ready", Priority: 4, Created: 2},
		},
		Containment: []Containment{
			{ChildID: "issue-z", ParentID: "issue-m"},
			{ChildID: "issue-a", ParentID: "issue-m"},
		},
		Dependencies: []Dependency{
			{ChildID: "issue-z", ParentID: "issue-m"},
			{ChildID: "issue-a", ParentID: "issue-m"},
		},
		Provenance: testProvenance("board-1"),
		Selection:  WholeBoard(),
	})
	require.NoError(t, err)

	boardPage := renderedBody(t, rendered.Files, "README.md")
	require.Contains(t, boardPage, "| [issue-m]")
	require.Contains(t, boardPage, "| [issue-z]")
	require.Contains(t, boardPage, "| [issue-a]")
	assert.Less(t,
		strings.Index(boardPage, "| [issue-m]"),
		strings.Index(boardPage, "| [issue-z]"),
	)
	assert.Less(t,
		strings.Index(boardPage, "| [issue-z]"),
		strings.Index(boardPage, "| [issue-a]"),
	)

	parentPage := renderedBody(t, rendered.Files, "issues/issue-m.md")
	require.Contains(t, parentPage, "[issue-z: Older]")
	require.Contains(t, parentPage, "[issue-a: Newer]")
	assert.Less(t,
		strings.Index(parentPage, "[issue-z: Older]"),
		strings.Index(parentPage, "[issue-a: Newer]"),
	)
}

func TestRenderIssueWithDescendantsIsCollectionEntrypoint(t *testing.T) {
	rendered, err := render(Snapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-root", Title: "Root task", Type: "task", Status: "ready"},
			{ID: "an-child", Title: "Child workstream", Type: "workstream", Status: "in_progress"},
			{ID: "an-leaf", Title: "Leaf", Type: "task", Status: "cancelled"},
		},
		Containment: []Containment{
			{ChildID: "an-child", ParentID: "an-root"},
			{ChildID: "an-leaf", ParentID: "an-child"},
		},
		Provenance: testProvenance("board-1"),
		Selection:  SelectedIssues("an-root"),
	})
	require.NoError(t, err)

	root := renderedBody(t, rendered.Files, "issues/an-root.md")
	assert.Contains(t, root, "| In progress | 1 |\n| Waiting | 0 |\n| Closed | 0 |\n| Cancelled | 1 |")
	assert.Contains(t, root, "- [an-child: Child workstream](../issues/an-child.md)\n  - [an-leaf: Leaf](../issues/an-leaf.md)")
	leaf := renderedBody(t, rendered.Files, "issues/an-leaf.md")
	assert.NotContains(t, leaf, "Descendant lifecycle summary")
	assert.NotContains(t, leaf, "Descendant containment tree")
}

func TestRenderNamedIssuesOnlyOmitsDescendantSummary(t *testing.T) {
	rendered, err := render(Snapshot{
		BoardID: "board-1",
		Issues: []Issue{
			{ID: "an-parent", Title: "Parent", Type: "task", Status: "ready"},
			{ID: "an-child", Title: "Child", Type: "task", Status: "closed"},
		},
		Containment: []Containment{{ChildID: "an-child", ParentID: "an-parent"}},
		Provenance:  testProvenance("board-1"),
		Selection:   NamedIssuesOnly("an-parent", "an-child"),
	})
	require.NoError(t, err)

	parent := renderedBody(t, rendered.Files, "issues/an-parent.md")
	assert.NotContains(t, parent, "Descendant lifecycle summary")
	assert.NotContains(t, parent, "Descendant containment tree")
}

func TestRenderLogEntriesUseStableExplicitAnchors(t *testing.T) {
	const logID = issue.LogID("log_0123456789abcdef0123456789abcdef")
	rendered, err := render(Snapshot{
		BoardID: "board-1",
		Issues:  []Issue{{ID: "an-issue", Title: "Issue"}},
		LogEntries: []LogEntry{{
			ID: logID, IssueID: "an-issue", Kind: "post",
			Author: new("captain"), Committer: new("captain"),
			Body: "Entry body.", Created: new(int64(1)),
		}, {
			ID:      "log_11111111111111111111111111111111",
			IssueID: "an-issue", Kind: "state_snapshot",
			Body: "State body.", NextAction: new("Planned action."),
		}},
		Provenance: testProvenance("board-1"),
		Selection:  NamedIssuesOnly("an-issue"),
	})
	require.NoError(t, err)

	assert.Contains(t, renderedBody(t, rendered.Files, "issues/an-issue.md"),
		"<a id=\""+logID.String()+"\"></a>\n"+
			"### captain at 1970-01-01T00:00:01Z (log entry "+logID.String()+")\n")
	assert.Contains(
		t,
		renderedBody(t, rendered.Files, "issues/an-issue.md"),
		"### State snapshot by unknown actor, committed by unknown actor "+
			"at unknown time (log entry "+
			"log_11111111111111111111111111111111)\n",
	)
	assert.Contains(
		t,
		renderedBody(t, rendered.Files, "issues/an-issue.md"),
		"State body.\n\n**Planned next action**\n\nPlanned action.",
	)
}

func TestRenderRejectsUnsafeIssuePath(t *testing.T) {
	_, err := render(Snapshot{
		BoardID: "board-1", Issues: []Issue{{ID: "../escape", Title: "Escape"}},
		Provenance: testProvenance("board-1"),
		Selection:  WholeBoard(),
	})
	assert.EqualError(t, err, `render issue "../escape": issue ID is not safe for a dump path`)
}

func renderedPaths(files []*GeneratedFile) []string {
	paths := make([]string, len(files))
	for index, file := range files {
		paths[index] = file.Path()
	}
	return paths
}

func renderedIdentities(files []*GeneratedFile) []string {
	identities := make([]string, len(files))
	for index, file := range files {
		identities[index] = file.Identity()
	}
	return identities
}

func renderedBody(t *testing.T, files []*GeneratedFile, path string) string {
	t.Helper()
	for _, file := range files {
		if file.Path() == path {
			_, body, err := decodeOwnedFile(generatedFileContent(t, file))
			require.NoError(t, err)
			return string(body)
		}
	}
	require.FailNow(t, "rendered file not found", path)
	return ""
}

func renderedContents(t *testing.T, files []*GeneratedFile) [][]byte {
	t.Helper()
	contents := make([][]byte, len(files))
	for index, file := range files {
		contents[index] = generatedFileContent(t, file)
	}
	return contents
}

func generatedFileContent(t *testing.T, file *GeneratedFile) []byte {
	t.Helper()
	reader, err := file.Open()
	require.NoError(t, err)
	content, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	require.NoError(t, errors.Join(readErr, closeErr))
	return content
}
