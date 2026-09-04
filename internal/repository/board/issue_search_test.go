package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/searchquery"
)

func TestRepositorySearchIssuesTracksEveryRecordFamily(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "search-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	queries := issue.NewQueries(repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title:    "Title marigold",
			Type:     "task",
			Priority: 2,
			Summary:  "Summary periwinkle",
			Details:  `Details chrysanthemum says "hello world".`,
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetStateRequest{
			IssueID:    "search-1",
			Text:       "State heliotrope",
			NextAction: "Continue with celosia",
		},
	)
	require.NoError(t, err)
	logEntry, err := recorder.AddLogEntry(
		t.Context(),
		issue.NewInvocation("captain"),
		record.AddLogEntryRequest{
			IssueID: "search-1",
			Body:    "Log nasturtium",
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetResultRequest{
			IssueID: "search-1",
			Body:    "Result snapdragon",
		},
	)
	require.NoError(t, err)

	tests := []struct {
		query      string
		field      issue.SearchField
		wantRecord *issue.LogID
	}{
		{query: "marigold", field: issue.SearchFieldTitle},
		{query: "periwinkle", field: issue.SearchFieldSummary},
		{query: "chrysanthemum", field: issue.SearchFieldDetails},
		{query: "heliotrope", field: issue.SearchFieldState},
		{query: "celosia", field: issue.SearchFieldState},
		{query: "snapdragon", field: issue.SearchFieldResult},
		{query: "nasturtium", field: issue.SearchFieldLog, wantRecord: &logEntry.LogEntry.ID},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
				Query: mustSearchQuery(t, test.query),
			})
			require.NoError(t, err)
			require.Equal(t, 1, result.Total)
			require.Len(t, result.Matches, 1)
			match := result.Matches[0]
			assert.Equal(t, "search-1", match.Summary.Issue.ID)
			assert.Equal(t, []issue.SearchField{test.field}, match.MatchedFields)
			assert.Equal(t, test.field, match.Excerpt.Field)
			assert.Equal(t, test.wantRecord, match.Excerpt.RecordID)
			assert.Contains(t, match.Excerpt.Text, "[")
		})
	}

	for _, expression := range []string{
		`"Title marigold"`,
		"chrysan*",
		"marigold OR snapdragon",
		"marigold NOT pepper",
	} {
		result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
			Query: mustSearchQuery(t, expression),
		})
		require.NoError(t, err, expression)
		assert.Equal(t, 1, result.Total, expression)
	}
	literal, err := searchquery.Literal(`says "hello world"`)
	require.NoError(t, err)
	result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
		Query: literal,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
}

func TestRepositorySearchIssuesMaintainsDerivedDocuments(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "search-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	queries := issue.NewQueries(repository)

	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Original saffron", Type: "task", Priority: 2,
			Summary: "Original sumac", Details: "Original anise",
		},
	)
	require.NoError(t, err)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetStateRequest{IssueID: "search-1", Text: "Original caraway"},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetResultRequest{IssueID: "search-1", Body: "Original cumin"},
	)
	require.NoError(t, err)

	title := "Replacement turmeric"
	summary := "Replacement coriander"
	details := "Replacement cardamom"
	_, err = planner.EditIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.EditIssueRequest{
			ID: "search-1", Title: &title,
			Summary: &summary, SummarySet: true,
			Details: &details, DetailsSet: true,
		},
	)
	require.NoError(t, err)
	_, err = recorder.ClearState(
		t.Context(),
		issue.NewInvocation("captain"),
		record.ClearStateRequest{IssueID: "search-1"},
	)
	require.NoError(t, err)
	_, err = recorder.SetResult(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetResultRequest{IssueID: "search-1", Body: "Replacement pepper"},
	)
	require.NoError(t, err)

	for _, stale := range []string{"saffron", "sumac", "anise", "caraway", "cumin"} {
		result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
			Query: mustSearchQuery(t, stale),
		})
		require.NoError(t, err)
		assert.Zero(t, result.Total, stale)
	}
	for _, current := range []string{"turmeric", "coriander", "cardamom", "pepper"} {
		result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
			Query: mustSearchQuery(t, current),
		})
		require.NoError(t, err)
		assert.Equal(t, 1, result.Total, current)
	}
}

func TestRepositorySearchIssuesRanksAndFiltersIssueMatches(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "search-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	executor := execution.NewExecutor(repository, repository)
	queries := issue.NewQueries(repository)

	repository.clock = fixedClock{now: time.Unix(1_700_000_000, 0).UTC()}
	_, err := planner.CreateIssue(
		t.Context(), issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Parent", Type: "workstream", Priority: 1,
		},
	)
	require.NoError(t, err)
	repository.clock = fixedClock{now: time.Unix(1_700_000_100, 0).UTC()}
	_, err = planner.CreateIssue(
		t.Context(), issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Quasar in title", Type: "task", Priority: 1,
			Parent: "search-1", Labels: []string{"alpha", "selected"},
		},
	)
	require.NoError(t, err)
	repository.clock = fixedClock{now: time.Unix(1_700_000_200, 0).UTC()}
	_, err = planner.CreateIssue(
		t.Context(), issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Other child", Type: "task", Priority: 3,
			Parent: "search-1", Summary: "Quasar in summary",
			Labels: []string{"alpha"},
		},
	)
	require.NoError(t, err)
	_, err = recorder.AddLogEntry(
		t.Context(), issue.NewInvocation("captain"),
		record.AddLogEntryRequest{IssueID: "search-3", Body: "Quasar again"},
	)
	require.NoError(t, err)
	_, err = executor.CloseIssues(
		t.Context(), issue.NewInvocation("captain"),
		execution.CloseIssuesRequest{IDs: []string{"search-3"}},
	)
	require.NoError(t, err)

	result, err := queries.SearchIssues(t.Context(), issue.SearchRequest{
		Query: mustSearchQuery(t, "quasar"),
	})
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	require.Len(t, result.Matches, 2)
	assert.Equal(t, "search-2", result.Matches[0].Summary.Issue.ID)
	assert.Equal(t, []issue.SearchField{
		issue.SearchFieldSummary,
		issue.SearchFieldLog,
	}, result.Matches[1].MatchedFields)

	createdSince := time.Unix(1_700_000_050, 0).UTC()
	createdBefore := time.Unix(1_700_000_150, 0).UTC()
	result, err = queries.SearchIssues(t.Context(), issue.SearchRequest{
		Query: mustSearchQuery(t, "quasar"), UnderID: "search-1",
		Type: "task", LabelsAll: []string{"alpha", "selected"},
		Statuses:     []string{issue.StatusReady.String()},
		CreatedSince: &createdSince, CreatedBefore: &createdBefore,
		Fields: []issue.SearchField{issue.SearchFieldTitle},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, "search-2", result.Matches[0].Summary.Issue.ID)

	closedSince := time.Unix(1_700_000_150, 0).UTC()
	closedBefore := time.Unix(1_700_000_250, 0).UTC()
	result, err = queries.SearchIssues(t.Context(), issue.SearchRequest{
		Query:       mustSearchQuery(t, "quasar"),
		Statuses:    []string{issue.StatusClosed.String()},
		ClosedSince: &closedSince, ClosedBefore: &closedBefore,
		Sort: "created", Reverse: true, Limit: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
	require.Len(t, result.Matches, 1)
	assert.Equal(t, "search-3", result.Matches[0].Summary.Issue.ID)
}

func mustSearchQuery(t *testing.T, value string) searchquery.Query {
	t.Helper()
	query, err := searchquery.Parse(value)
	require.NoError(t, err)
	return query
}
