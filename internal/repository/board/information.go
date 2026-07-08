package board

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/must"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
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
	queries := query.New(view)
	total, err := queries.BoardCountInventoryIssues(ctx, r.boardID.String())
	if err != nil {
		return IssueInventory{}, fmt.Errorf("count board issues: %w", err)
	}
	out := IssueInventory{Total: int(total)}
	for _, status := range issue.ValidStatuses() {
		count, err := queries.BoardCountInventoryIssuesByStatus(
			ctx,
			query.BoardCountInventoryIssuesByStatusParams{
				BoardID: r.boardID.String(),
				Status:  status.String(),
			},
		)
		if err != nil {
			return IssueInventory{}, fmt.Errorf(
				"count %s issues: %w",
				status.String(),
				err,
			)
		}
		out.ByStatus = append(out.ByStatus, IssueStatusCount{
			Status: status,
			Count:  int(count),
		})
	}
	return out, nil
}
