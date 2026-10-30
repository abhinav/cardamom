package boardhistory

import (
	"context"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	history "go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// ReadEvent reads one previously accepted journal event by causal identity.
// It decodes one event at a time and stops at the requested ordinal. Mutation
// integrity was established before publication; this is a read of that retained
// state, not another import verification pass.
func ReadEvent(ctx context.Context, reader *query.Queries, boardID board.ID, id history.EventID) (history.Event, error) {
	encoded, err := reader.BoardHistoryReadMarker(ctx, query.BoardHistoryReadMarkerParams{
		BoardID: boardID.String(), WriterUid: id.Writer.Bytes(), Sequence: int64(id.Sequence),
	})
	if err != nil {
		return history.Event{}, err
	}
	marker, err := history.DecodeMarker(encoded)
	if err != nil {
		return history.Event{}, err
	}
	for retained, err := range history.MutationEvents(marker, func(index int64) ([]byte, error) {
		return reader.BoardHistoryClonePart(ctx, query.BoardHistoryClonePartParams{
			BoardID: boardID.String(), WriterUid: id.Writer.Bytes(), Sequence: int64(id.Sequence), PartIndex: index,
		})
	}) {
		if err != nil {
			return history.Event{}, err
		}
		if retained.ID.Ordinal == id.Ordinal {
			return retained.Event, nil
		}
	}
	return history.Event{}, fmt.Errorf("retained mutation has no event at ordinal %d", id.Ordinal)
}
