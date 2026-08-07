package issue

import (
	"cmp"
	"slices"
)

// BoardIssueSummary retains board ownership while issue summaries from
// independent board queries are combined.
type BoardIssueSummary struct {
	// BoardID is the stable identity of the board that owns Summary.
	BoardID string

	// Summary is one board-scoped issue query result.
	Summary Summary
}

// Order is the normalized ordering contract shared by board-local and
// aggregate issue queries.
type Order struct {
	// Key is empty for the creation-time default order.
	Key string

	// Reverse applies descending direction to every selected order component.
	Reverse bool
}

// NormalizeOrder returns the established list order for request. An
// unknown non-empty sort uses the fixed default order and ignores Reverse.
func NormalizeOrder(request ListRequest) Order {
	switch request.Sort {
	case "":
		return Order{Reverse: request.Reverse}
	case "priority", "created", "updated", "closed", "title", "type":
		return Order{Key: request.Sort, Reverse: request.Reverse}
	default:
		return Order{}
	}
}

// OrderSummaries applies the ListRequest ordering and global limit
// to summaries gathered from multiple boards. Creation time precedes board and
// issue identity tie-breakers so identity does not become presentation order.
func OrderSummaries(request ListRequest, values []BoardIssueSummary) []BoardIssueSummary {
	ordered := slices.Clone(values)
	issueOrder := NormalizeOrder(request)
	slices.SortStableFunc(ordered, func(left, right BoardIssueSummary) int {
		order := compareIssueSummary(issueOrder.Key, left.Summary.Issue, right.Summary.Issue)
		if issueOrder.Reverse {
			order = -order
		}
		if order != 0 {
			return order
		}
		if issueOrder.Key != "" && issueOrder.Key != "created" {
			order = cmp.Compare(left.Summary.Issue.Created, right.Summary.Issue.Created)
			if issueOrder.Reverse {
				order = -order
			}
			if order != 0 {
				return order
			}
		}
		if order = cmp.Compare(left.BoardID, right.BoardID); order != 0 {
			return order
		}
		return cmp.Compare(left.Summary.Issue.ID, right.Summary.Issue.ID)
	})
	if request.Limit > 0 && len(ordered) > request.Limit {
		ordered = ordered[:request.Limit]
	}
	return ordered
}

func compareIssueSummary(key string, left, right Issue) int {
	switch key {
	case "priority":
		return cmp.Compare(left.Priority, right.Priority)
	case "created":
		return cmp.Compare(left.Created, right.Created)
	case "updated":
		return cmp.Compare(left.Updated, right.Updated)
	case "closed":
		return compareOptionalInt64(left.Closed, right.Closed)
	case "title":
		return cmp.Compare(left.Title, right.Title)
	case "type":
		return cmp.Compare(left.Type, right.Type)
	case "":
		return cmp.Compare(left.Created, right.Created)
	default:
		panic("board: unnormalized issue order " + key)
	}
}

func compareOptionalInt64(left, right *int64) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	default:
		return cmp.Compare(*left, *right)
	}
}
