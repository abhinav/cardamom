package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/searchquery"
	"go.uber.org/mock/gomock"
)

func TestSearchCommandPassesQueryAndMetadataFilters(t *testing.T) {
	var stdout, stderr bytes.Buffer
	createdSince := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	createdBefore := createdSince.Add(24 * time.Hour)
	closedSince := createdBefore
	closedBefore := closedSince.Add(24 * time.Hour)
	query, err := searchquery.Parse(`alpha "beta gamma" OR delta*`)
	require.NoError(t, err)
	want := issue.SearchRequest{
		Query: query,
		Fields: []issue.SearchField{
			issue.SearchFieldTitle,
			issue.SearchFieldLog,
		},
		UnderID: "an-parent", Statuses: []string{"ready", "closed"},
		Assignee: new("worker"), Type: "task",
		LabelsAll: []string{"area:cli"}, LabelsAny: []string{"phase:a"},
		LabelsNone:   []string{"archived"},
		CreatedSince: &createdSince, CreatedBefore: &createdBefore,
		ClosedSince: &closedSince, ClosedBefore: &closedBefore,
		Sort: "updated", Reverse: true, Limit: 9,
	}
	operation := NewMockSearchIssuesOperation(gomock.NewController(t))
	operation.EXPECT().SearchIssues(gomock.Any(), want).Return(issue.SearchResult{}, nil)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*SearchIssuesOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{
		"search", `alpha "beta gamma" OR delta*`,
		"--in", "title,log", "--under", "an-parent",
		"--status", "ready,closed", "--assignee", "worker", "--type", "task",
		"--label", "+area:cli", "--label", "-archived", "--label-any", "phase:a",
		"--created-since", createdSince.Format(time.RFC3339),
		"--created-before", createdBefore.Format(time.RFC3339),
		"--closed-since", closedSince.Format(time.RFC3339),
		"--closed-before", closedBefore.Format(time.RFC3339),
		"--sort", "updated", "--reverse", "--limit", "9",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	assert.Equal(t, "0 matches\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestSearchCommandEmitsOneJSONEnvelope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	logID := issue.LogID("log_11111111111111111111111111111111")
	result := issue.SearchResult{
		Total: 3,
		Matches: []issue.SearchMatch{{
			Summary: issue.Summary{
				Issue: issue.Issue{
					ID: "an-1", Title: "Found", Type: "task", Lifecycle: "open",
					Status: "ready", Priority: 2, Created: 10, Updated: 11, Revision: 3,
				},
				Labels: []string{"area:search"},
			},
			MatchedFields: []issue.SearchField{issue.SearchFieldTitle, issue.SearchFieldLog},
			Excerpt: issue.SearchExcerpt{
				Field: issue.SearchFieldLog, RecordID: &logID,
				Text: "A [matching] excerpt",
			},
		}},
	}
	query, err := searchquery.Parse("matching")
	require.NoError(t, err)
	operation := NewMockSearchIssuesOperation(gomock.NewController(t))
	operation.EXPECT().SearchIssues(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, got issue.SearchRequest) (issue.SearchResult, error) {
			assert.Equal(t, query.Expression(), got.Query.Expression())
			assert.Equal(t, "relevance", got.Sort)
			assert.Equal(t, 20, got.Limit)
			return result, nil
		},
	)
	app := newInspectionApplication(
		t,
		testConfig(&stdout, &stderr),
		kong.BindTo(operation, (*SearchIssuesOperation)(nil)),
	)

	exitCode := app.Run(t.Context(), []string{"--json", "search", "matching"})

	assert.Equal(t, ExitSuccess, exitCode)
	var got map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &got))
	assert.Equal(t, float64(3), got["total"])
	matches := got["matches"].([]any)
	require.Len(t, matches, 1)
	match := matches[0].(map[string]any)
	assert.Equal(t, "an-1", match["id"])
	assert.Equal(t, []any{"title", "log"}, match["matched_in"])
	excerpt := match["excerpt"].(map[string]any)
	assert.Equal(t, "log", excerpt["field"])
	assert.Equal(t, logID.String(), excerpt["record_id"])
	assert.Empty(t, stderr.String())
}

func TestSearchCommandRejectsInvalidCombinationsAndTimes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "ReverseRelevance", args: []string{"search", "term", "--reverse"}, want: "--reverse cannot be used with relevance order"},
		{name: "AssigneeConflict", args: []string{"search", "term", "--assignee", "worker", "--no-assignee"}, want: "cannot be combined"},
		{name: "DuplicateField", args: []string{"search", "term", "--in", "title", "--in", "title"}, want: "duplicate --in field"},
		{name: "InvalidTime", args: []string{"search", "term", "--created-since", "yesterday"}, want: "must be an RFC 3339 time"},
		{name: "InvalidTimeRange", args: []string{"search", "term", "--created-since", "2026-09-02T00:00:00Z", "--created-before", "2026-09-01T00:00:00Z"}, want: "must be before"},
		{name: "InvalidQuery", args: []string{"search", "term OR"}, want: "invalid search query"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			operation := NewMockSearchIssuesOperation(gomock.NewController(t))
			app := newInspectionApplication(
				t,
				testConfig(&stdout, &stderr),
				kong.BindTo(operation, (*SearchIssuesOperation)(nil)),
			)

			exitCode := app.Run(t.Context(), test.args)

			assert.Equal(t, ExitUsage, exitCode)
			assert.Empty(t, stdout.String())
			assert.Contains(t, stderr.String(), test.want)
		})
	}
}
