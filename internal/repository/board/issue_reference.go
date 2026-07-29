package board

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ResolveIssueReferences returns issue IDs known to this repository's board.
func (r *Repository) ResolveIssueReferences(
	ctx context.Context,
	issueIDs []issue.ID,
) (_ []issue.ID, err error) {
	if len(issueIDs) == 0 {
		return []issue.ID{}, nil
	}

	view, err := r.store.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin issue reference resolution: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	ids := make([]string, len(issueIDs))
	for index, id := range issueIDs {
		ids[index] = id.String()
	}
	rows, err := query.New(view).BoardResolveIssueReferences(
		ctx,
		query.BoardResolveIssueReferencesParams{
			BoardID:  r.boardID.String(),
			IssueIDs: ids,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("select issue references: %w", err)
	}

	references := make([]issue.ID, 0, len(issueIDs))
	for _, row := range rows {
		parsedIssueID, err := issue.NewID(row)
		if err != nil {
			return nil, fmt.Errorf("parse issue reference ID: %w", err)
		}
		references = append(references, parsedIssueID)
	}
	return references, nil
}
