package board

import (
	"context"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

var _ issue.MarkerReader = (*Locator)(nil)

// ReadMarker reads a retired public ID without selecting a board.
// Local candidate titles and IDs come from the same retained store view;
// unavailable candidates retain their saved replacement ID.
func (l *Locator) ReadMarker(ctx context.Context, id issue.ID) (out *issue.ReferenceMarker, err error) {
	view, err := l.store.View(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, view.Done()) }()
	return readReferenceMarker(ctx, query.New(view), id)
}

func readReferenceMarker(ctx context.Context, queries *query.Queries, id issue.ID) (*issue.ReferenceMarker, error) {
	exists, err := queries.BoardReferenceMarkerExists(ctx, id.String())
	if err != nil {
		return nil, fmt.Errorf("read retired issue ID: %w", err)
	}
	if !exists {
		return nil, nil
	}
	rows, err := queries.BoardReadReferenceCandidates(ctx, id.String())
	if err != nil {
		return nil, fmt.Errorf("read issue reference candidates: %w", err)
	}
	marker := &issue.ReferenceMarker{ID: id, Candidates: make([]issue.MarkerCandidate, 0, len(rows))}
	for _, row := range rows {
		uid, err := issue.ParseUID(row.IssueUid)
		if err != nil {
			return nil, fmt.Errorf("read candidate identity: %w", err)
		}
		publicID := row.ReplacementID
		if row.CurrentID != nil {
			publicID = *row.CurrentID
		}
		candidateID, err := issue.NewID(publicID)
		if err != nil {
			return nil, fmt.Errorf("read candidate public ID: %w", err)
		}
		marker.Candidates = append(marker.Candidates, issue.MarkerCandidate{
			UID: uid, BoardID: row.BoardID, ID: candidateID, Title: row.Title, Available: row.CurrentID != nil,
		})
	}
	return marker, nil
}

// cloneReferenceMarkers carries every candidate of a marker associated with
// this board, including unavailable candidates on other boards. Bounded pages
// keep copying independent of the number of retired references in the store.
func cloneReferenceMarkers(ctx context.Context, source, destination *query.Queries, boardID string) error {
	const pageSize = 128
	for offset := int64(0); ; offset += pageSize {
		rows, err := source.BoardCloneReferenceCandidates(ctx, query.BoardCloneReferenceCandidatesParams{
			BoardID: boardID, PageSize: pageSize, PageOffset: offset,
		})
		if err != nil {
			return fmt.Errorf("read cloned reference markers: %w", err)
		}
		for _, row := range rows {
			if err := destination.BoardInsertReferenceMarker(ctx, row.MarkerID); err != nil {
				return fmt.Errorf("retain cloned marker %q: %w", row.MarkerID, err)
			}
			if err := destination.BoardInsertReferenceCandidate(ctx, query.BoardInsertReferenceCandidateParams(row)); err != nil {
				return fmt.Errorf("retain cloned marker candidate: %w", err)
			}
		}
		if len(rows) < pageSize {
			return nil
		}
	}
}
