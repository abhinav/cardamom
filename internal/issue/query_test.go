package issue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestQueriesExposeFiniteIssueReads(t *testing.T) {
	t.Parallel()

	expectedIssues := []Summary{{Issue: Issue{ID: "an-1"}}}
	expectedView := View{Detail: Detail{Issue: Issue{ID: "an-1"}}}
	reader := NewMockQueryReader(gomock.NewController(t))
	reader.EXPECT().ResolveExternalKey(gomock.Any(), "source:one").Return(ID("an-1"), nil)
	reader.EXPECT().ListIssues(gomock.Any(), ListRequest{
		LabelsAll: []string{"protocol"},
	}).Return(expectedIssues, nil)
	reader.EXPECT().ListIssuesSnapshot(gomock.Any(), ListRequest{Limit: 1}).Return(
		ListSnapshot{Issues: expectedIssues, Total: 1}, nil,
	)
	reader.EXPECT().ReadIssue(gomock.Any(), ReadRequest{IssueID: "an-1"}).Return(expectedView, nil)
	queries := NewQueries(reader)

	resolved, err := queries.ResolveExternalKey(t.Context(), "source:one")
	require.NoError(t, err)
	assert.Equal(t, ID("an-1"), resolved)

	issues, err := queries.ListIssues(t.Context(), ListRequest{LabelsAll: []string{"protocol"}})
	require.NoError(t, err)
	assert.Equal(t, []Summary{{Issue: Issue{ID: "an-1"}}}, issues)

	snapshot, err := queries.ListIssuesSnapshot(t.Context(), ListRequest{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, ListSnapshot{Issues: expectedIssues, Total: 1}, snapshot)

	view, err := queries.ReadIssue(t.Context(), ReadRequest{IssueID: "an-1"})
	require.NoError(t, err)
	assert.Equal(t, View{Detail: Detail{Issue: Issue{ID: "an-1"}}}, view)
}

func TestQueriesRejectInvalidLabelSelectors(t *testing.T) {
	t.Parallel()

	queries := NewQueries(NewMockQueryReader(gomock.NewController(t)))

	_, err := queries.ListIssues(t.Context(), ListRequest{
		LabelsNone: []string{"-archived"},
	})
	require.ErrorContains(t, err, "label cannot start with + or -")

	_, err = queries.ListIssuesSnapshot(t.Context(), ListRequest{
		LabelsAny: []string{" "},
	})
	require.ErrorContains(t, err, "label cannot be empty")
}
