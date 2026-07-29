package issue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueriesExposeFiniteIssueReads(t *testing.T) {
	t.Parallel()

	reader := &queryReaderStub{
		issues: []Summary{{Issue: Issue{ID: "an-1"}}},
		view:   View{Detail: Detail{Issue: Issue{ID: "an-1"}}},
	}
	queries := NewQueries(reader)

	issues, err := queries.ListIssues(t.Context(), ListRequest{LabelsAll: []string{"protocol"}})
	require.NoError(t, err)
	assert.Equal(t, reader.issues, issues)

	snapshot, err := queries.ListIssuesSnapshot(t.Context(), ListRequest{Limit: 1})
	require.NoError(t, err)
	assert.Equal(t, ListSnapshot{Issues: reader.issues, Total: 1}, snapshot)

	view, err := queries.ReadIssue(t.Context(), ReadRequest{IssueID: "an-1"})
	require.NoError(t, err)
	assert.Equal(t, reader.view, view)
}

func TestQueriesRejectInvalidLabelSelectors(t *testing.T) {
	t.Parallel()

	queries := NewQueries(new(queryReaderStub))

	_, err := queries.ListIssues(t.Context(), ListRequest{
		LabelsNone: []string{"-archived"},
	})
	require.ErrorContains(t, err, "label cannot start with + or -")

	_, err = queries.ListIssuesSnapshot(t.Context(), ListRequest{
		LabelsAny: []string{" "},
	})
	require.ErrorContains(t, err, "label cannot be empty")
}

type queryReaderStub struct {
	issues []Summary
	view   View
}

func (s *queryReaderStub) ListIssues(context.Context, ListRequest) ([]Summary, error) {
	return s.issues, nil
}

func (s *queryReaderStub) ListIssuesSnapshot(context.Context, ListRequest) (ListSnapshot, error) {
	return ListSnapshot{Issues: s.issues, Total: len(s.issues)}, nil
}

func (s *queryReaderStub) ReadIssue(context.Context, ReadRequest) (View, error) {
	return s.view, nil
}
