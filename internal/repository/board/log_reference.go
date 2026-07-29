package board

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ResolveLogReferences returns ownership metadata for log IDs known to this
// repository's board.
func (r *Repository) ResolveLogReferences(
	ctx context.Context,
	logIDs []issue.LogID,
) (_ []issue.LogReference, err error) {
	if len(logIDs) == 0 {
		return []issue.LogReference{}, nil
	}

	view, err := r.store.View(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin log reference resolution: %w", err)
	}
	defer func() { err = errors.Join(err, view.Done()) }()

	ids := make([]string, len(logIDs))
	for index, id := range logIDs {
		ids[index] = id.String()
	}
	rows, err := query.New(view).BoardResolveLogReferences(
		ctx,
		query.BoardResolveLogReferencesParams{
			BoardID: r.boardID.String(),
			LogIDs:  ids,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("select log references: %w", err)
	}

	references := make([]issue.LogReference, 0, len(logIDs))
	for _, row := range rows {
		parsedLogID, err := issue.NewLogID(row.ID)
		if err != nil {
			return nil, fmt.Errorf("parse log reference ID: %w", err)
		}
		parsedIssueID, err := issue.NewID(row.IssueID)
		if err != nil {
			return nil, fmt.Errorf("parse log reference owner: %w", err)
		}
		references = append(references, issue.LogReference{
			LogID: parsedLogID, IssueID: parsedIssueID,
		})
	}
	return references, nil
}
