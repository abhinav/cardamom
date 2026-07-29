package issue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderSummariesAppliesGlobalOrderLimitAndTieBreaker(t *testing.T) {
	values := []BoardIssueSummary{
		{BoardID: "board-b", Summary: Summary{Issue: Issue{ID: "issue-z", Title: "Zulu", Priority: 1}}},
		{BoardID: "board-b", Summary: Summary{Issue: Issue{ID: "issue-b", Title: "Alpha", Priority: 1}}},
		{BoardID: "board-a", Summary: Summary{Issue: Issue{ID: "issue-a", Title: "Alpha", Priority: 1}}},
	}

	ordered := OrderSummaries(ListRequest{Sort: "title", Limit: 2}, values)
	require.Len(t, ordered, 2)
	assert.Equal(t, []string{"issue-a", "issue-b"}, []string{
		ordered[0].Summary.Issue.ID, ordered[1].Summary.Issue.ID,
	})
	assert.Equal(t, "issue-z", values[0].Summary.Issue.ID, "input remains unchanged")
}

func TestOrderSummariesIgnoresReverseForUnknownSort(t *testing.T) {
	values := []BoardIssueSummary{
		{BoardID: "board-b", Summary: Summary{Issue: Issue{ID: "issue-low", Priority: 4, Created: 1}}},
		{BoardID: "board-a", Summary: Summary{Issue: Issue{ID: "issue-high", Priority: 0, Created: 2}}},
	}

	ordered := OrderSummaries(ListRequest{Sort: "unknown", Reverse: true}, values)
	require.Len(t, ordered, 2)
	assert.Equal(t, []string{"issue-high", "issue-low"}, []string{
		ordered[0].Summary.Issue.ID, ordered[1].Summary.Issue.ID,
	})
}
