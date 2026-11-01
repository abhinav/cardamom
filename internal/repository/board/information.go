package board

import (
	"context"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// IssueStatusCount reports one derived issue-status population.
type IssueStatusCount struct {
	// Status is the derived issue status being counted.
	Status issue.Status

	// Count is the number of selected-board issues with Status.
	Count int
}

// IssueInventory reports the selected board's issue population.
type IssueInventory struct {
	// Total is the number of issues in the selected board.
	Total int

	// ByStatus reports counts in issue.ValidStatuses order.
	ByStatus []IssueStatusCount
}

// ReadIssueInventory reads board-owned issue counts through the caller's
// retained store snapshot.
func (r *Repository) ReadIssueInventory(
	ctx context.Context,
	view *store.View,
) (IssueInventory, error) {
	must.NotBeNilf(view, "board information View is required")
	presentation, err := r.readIssuePresentation(ctx, view)
	if err != nil {
		return IssueInventory{}, err
	}
	counts := make(map[string]int)
	for id := range presentation.IDs() {
		counts[presentation.Summary(id).Issue.Status]++
	}
	out := IssueInventory{Total: presentation.Len()}
	for _, status := range issue.ValidStatuses() {
		out.ByStatus = append(out.ByStatus, IssueStatusCount{Status: status, Count: counts[status.String()]})
	}
	return out, nil
}
