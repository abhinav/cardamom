package issueview

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"go.uber.org/mock/gomock"
)

func TestEncoderSummaryAndMarkdown(t *testing.T) {
	t.Parallel()

	renderer := NewMockMarkdownRenderer(gomock.NewController(t))
	renderer.EXPECT().RenderBoard(
		gomock.Any(), board.ID("board-test"), "", []string{"**diagnostic**"},
	).Return([]string{"<p>**diagnostic**</p>"}, nil)
	encoder := New(renderer)
	started := int64(1784376060)
	summary, err := encoder.Summary("board-test", issue.Summary{
		Issue: issue.Issue{
			ID: "an-1", Title: "Inspect relay", Type: "task",
			Lifecycle: "open", Status: "ready", Priority: 2,
			Created: 1784376000, Updated: 1784376030, StartedAt: &started,
			ActiveClaim: &issue.ActiveClaim{Actor: "scotty", StartedAt: started},
		},
		Labels: []string{"area:engineering"}, Blocked: true,
	})
	require.NoError(t, err)
	assert.Equal(t, privatev1.IssueStatus_ISSUE_STATUS_BLOCKED, summary.GetStatus())
	assert.Equal(t, "board-test", summary.GetBoardId())
	assert.Equal(t, "scotty", summary.GetActiveClaim().GetActor())
	assert.Equal(t, []string{"area:engineering"}, summary.GetLabels())

	markdown, err := encoder.Markdown(t.Context(), "board-test", "**diagnostic**")
	require.NoError(t, err)
	assert.Equal(t, "**diagnostic**", markdown.GetSource())
	assert.Equal(t, "<p>**diagnostic**</p>", markdown.GetRenderedHtml())
}

func TestEncoderDetailPreservesContextAndContainment(t *testing.T) {
	t.Parallel()

	renderer := NewMockMarkdownRenderer(gomock.NewController(t))
	encoder := New(renderer)
	summary := "Repair the relay."
	details := "Relay specifications."
	state := "Replace the relay coil."
	nextAction := "Test the replacement coil."
	result := "Relay repaired."
	boardDescription := "Engineering operations."
	ancestorSummary := "Ship operations."
	ancestorState := "Coordinate repairs."
	ancestorNextAction := "Review repair reports."
	rootID := "an-root"
	latestLogID := issue.LogID("cmt_12121212121212121212121212121212")
	markdownSources := []string{
		summary, details, state, nextAction, result, boardDescription,
		ancestorSummary, ancestorState, ancestorNextAction, "Ready.",
	}
	renderer.EXPECT().RenderBoard(
		gomock.Any(), board.ID("board-test"), "", markdownSources,
	).Return(markdownSources, nil)
	view, err := encoder.Detail(t.Context(), "board-test", issue.View{
		Detail: issue.Detail{
			Issue: issue.Issue{
				ID: "an-selected", Title: "Repair relay", Type: "task",
				Lifecycle: "open", Status: "ready", Priority: 1,
				Created: 1784376000, Updated: 1784376030,
				Summary: &summary, Details: &details, State: &state,
				NextAction: &nextAction,
			},
			Keys:          []string{" producer:z ", "producer:a"},
			CurrentResult: &issue.Result{IssueID: "an-selected", Body: result},
			LogSummary:    issue.LogSummary{Count: 2, LatestID: &latestLogID},
			Story: issue.Story{Containment: []issue.ContainmentNode{
				{Reference: testReference("an-root"), ParentID: nil},
				{Reference: testReference("an-selected"), ParentID: &rootID},
			}},
		},
		Context: &issue.Context{
			Board: issue.BoardDescription{Description: &boardDescription},
			Ancestors: []issue.ContextEntry{{Issue: issue.Issue{
				ID: "an-root", Title: "Root", Type: "workstream",
				Lifecycle: "open", Status: "ready", Priority: 1,
				Summary: &ancestorSummary, State: &ancestorState,
				NextAction: &ancestorNextAction,
			}}},
			DependencyResults: []issue.DependencyResult{{
				Issue: testReference("an-prerequisite"), Body: "Ready.",
			}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "Repair the relay.", view.GetSummary().GetSource())
	assert.Equal(t, "Relay specifications.", view.GetDetails().GetSource())
	assert.Equal(t, state, view.GetState().GetSource())
	assert.Equal(t, nextAction, view.GetNextAction().GetSource())
	assert.Equal(t, result, view.GetResult().GetSource())
	assert.Equal(t, []string{" producer:z ", "producer:a"}, view.GetExternalKeys())
	assert.Equal(t, uint32(2), view.GetLogCount())
	assert.Equal(t, "cmt_12121212121212121212121212121212", view.GetLatestLogId())
	require.Len(t, view.GetContainment().GetNodes(), 2)
	assert.True(t, view.GetContainment().GetNodes()[0].GetSelectedPath())
	assert.True(t, view.GetContainment().GetNodes()[1].GetSelectedPath())
	assert.Equal(t, uint32(1), view.GetContainment().GetNodes()[1].GetDepth())
	assert.Equal(t, "Engineering operations.", view.GetContext().GetBoardDescription().GetSource())
	assert.Equal(
		t,
		ancestorNextAction,
		view.GetContext().GetAncestors()[0].GetNextAction().GetSource(),
	)
	assert.Equal(t, "Ready.", view.GetContext().GetDependencyResults()[0].GetResult().GetSource())
}

func TestEncoderLogEntriesRenderInOneBoardBatch(t *testing.T) {
	renderer := NewMockMarkdownRenderer(gomock.NewController(t))
	renderer.EXPECT().RenderBoard(
		gomock.Any(), board.ID("board-test"), "",
		[]string{"First", "Second", "Continue", "Imported"},
	).Return([]string{"First", "Second", "Continue", "Imported"}, nil)
	encoder := New(renderer)

	logEntries, err := encoder.LogEntries(t.Context(), "board-test", []issue.LogEntry{
		{
			ID:      "cmt_11111111111111111111111111111111",
			IssueID: "an-1", Kind: "post",
			Author: new("one"), Committer: new("one"),
			Body: "First", Created: new(int64(1)),
		},
		{
			ID:      "cmt_22222222222222222222222222222222",
			IssueID: "an-1", Kind: "state_snapshot",
			Author: new("two"), Committer: new("reviewer"),
			Body: "Second", NextAction: new("Continue"),
			Created: new(int64(2)),
		},
		{
			ID:      "cmt_33333333333333333333333333333333",
			IssueID: "an-1", Kind: "state_snapshot",
			Body: "Imported",
		},
	})
	require.NoError(t, err)

	require.Len(t, logEntries, 3)
	require.NoError(t, protovalidate.Validate(logEntries[0]))
	assert.IsType(t, &privatev1.LogEntry_Post{}, logEntries[0].GetPayload())
	post := logEntries[0].GetPost()
	require.NotNil(t, post)
	assert.Equal(t, "First", post.GetBody().GetSource())
	assert.Equal(t, "one", post.GetActor())
	assert.Equal(t, int64(1), post.GetCreatedAt().GetSeconds())
	require.NoError(t, protovalidate.Validate(logEntries[1]))
	assert.IsType(
		t,
		&privatev1.LogEntry_StateSnapshot{},
		logEntries[1].GetPayload(),
	)
	stateSnapshot := logEntries[1].GetStateSnapshot()
	require.NotNil(t, stateSnapshot)
	assert.Equal(t, "Second", stateSnapshot.GetBody().GetSource())
	assert.Equal(t, "Continue", stateSnapshot.GetNextAction().GetSource())
	assert.Equal(t, "two", stateSnapshot.GetAuthor())
	assert.Equal(t, "reviewer", stateSnapshot.GetCommitter())
	assert.Equal(t, int64(2), stateSnapshot.GetCreatedAt().GetSeconds())
	require.NoError(t, protovalidate.Validate(logEntries[2]))
	imported := logEntries[2].GetStateSnapshot()
	require.NotNil(t, imported)
	assert.Equal(t, "Imported", imported.GetBody().GetSource())
	assert.Empty(t, imported.GetAuthor())
	assert.Empty(t, imported.GetCommitter())
	assert.Nil(t, imported.GetCreatedAt())
}

func TestEncoderRejectsInvalidPersistedValues(t *testing.T) {
	t.Parallel()

	encoder := New(NewMockMarkdownRenderer(gomock.NewController(t)))
	_, err := encoder.Summary("board-test", issue.Summary{Issue: issue.Issue{
		ID: "an-1", Type: "unknown", Lifecycle: "open", Status: "ready",
	}})
	assert.ErrorContains(t, err, "convert issue type")

	_, err = encoder.LogEntry(t.Context(), "board-test", issue.LogEntry{})
	assert.ErrorContains(t, err, "convert log ID")

	_, err = encoder.LogEntry(t.Context(), "board-test", issue.LogEntry{
		ID:      "log_11111111111111111111111111111111",
		IssueID: "an-1", Kind: "post", Body: "Post without metadata.",
	})
	assert.ErrorContains(t, err, "convert post author: required")

	parent := "an-child"
	_, err = containmentProjection("board-test", "an-child", []issue.ContainmentNode{
		{Reference: testReference("an-child"), ParentID: &parent},
	})
	assert.ErrorContains(t, err, "containment cycle")
}

func testReference(id string) issue.Reference {
	return issue.Reference{
		ID: id, Title: id, Type: "task", Status: "ready", Priority: 1,
	}
}
