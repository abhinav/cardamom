package issue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/searchquery"
	"go.uber.org/mock/gomock"
)

func TestQueriesSearchIssues(t *testing.T) {
	t.Parallel()

	query, err := searchquery.Parse("adapter")
	require.NoError(t, err)
	request := SearchRequest{
		Query: query, Fields: AllSearchFields(), LabelsAny: []string{"protocol"},
	}
	result := SearchResult{
		Total:   1,
		Matches: []SearchMatch{{Summary: Summary{Issue: Issue{ID: "an-1"}}}},
	}
	reader := NewMockQueryReader(gomock.NewController(t))
	reader.EXPECT().SearchIssues(gomock.Any(), request).Return(result, nil)

	got, err := NewQueries(reader).SearchIssues(t.Context(), SearchRequest{
		Query: query, LabelsAny: []string{"protocol"},
	})
	require.NoError(t, err)
	assert.Equal(t, result, got)
}

func TestQueriesRejectInvalidSearchRequest(t *testing.T) {
	t.Parallel()

	queries := NewQueries(NewMockQueryReader(gomock.NewController(t)))
	query, err := searchquery.Parse("adapter")
	require.NoError(t, err)

	_, err = queries.SearchIssues(t.Context(), SearchRequest{
		Query: query, Fields: []SearchField{SearchFieldTitle, SearchFieldTitle},
	})
	assert.ErrorContains(t, err, `duplicate search field "title"`)

	_, err = queries.SearchIssues(t.Context(), SearchRequest{
		Query: query, LabelsAll: []string{"-invalid"},
	})
	assert.ErrorContains(t, err, "label cannot start with + or -")
}

func TestNewSearchField(t *testing.T) {
	for _, value := range []string{"title", "summary", "details", "state", "result", "log"} {
		field, err := NewSearchField(value)
		require.NoError(t, err)
		assert.Equal(t, value, field.String())
	}

	_, err := NewSearchField("comments")
	assert.ErrorContains(t, err, `unsupported search field "comments"`)
}
