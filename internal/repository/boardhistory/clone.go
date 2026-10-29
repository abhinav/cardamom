package boardhistory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	history "go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// CloneV1 retains a board's existing mutation bytes and tips in a destination
// transaction. The caller owns both scopes and must retain the referenced blob
// descriptors before calling. No events are replayed or authored by the clone.
// Failure requires the destination transaction to roll back.
func CloneV1(
	ctx context.Context,
	source query.DBTX,
	destination query.DBTX,
	boardID board.ID,
) (int, error) {
	reader := query.New(source)
	writer := query.New(destination)
	afterWriter := []byte{}
	var afterSequence int64
	var previous history.Digest
	tips := make(map[board.WriterUID]history.Tip)
	count := 0
	for {
		row, err := reader.BoardHistoryNextCloneCommit(ctx,
			query.BoardHistoryNextCloneCommitParams{
				BoardID: boardID.String(), AfterWriterUid: afterWriter,
				AfterSequence: afterSequence,
			})
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read source mutation: %w", err)
		}
		marker, err := history.DecodeMarker(row.MarkerBytes)
		if err != nil {
			return 0, fmt.Errorf("decode source mutation: %w", err)
		}
		header := marker.Header()
		writerUID, err := board.ParseWriterUID(row.WriterUid)
		if err != nil {
			return 0, err
		}
		if header.Board != boardID || header.Writer != writerUID ||
			header.Sequence != uint64(row.Sequence) {
			return 0, errors.New("source mutation identity does not match its marker")
		}
		if _, found := tips[writerUID]; !found {
			afterSequence = 0
			previous = history.Digest{}
		}
		if row.Sequence != afterSequence+1 || header.Previous != previous {
			return 0, errors.New("source mutation writer chain is incomplete")
		}
		for _, tip := range header.Frontier {
			found, err := reader.StoreCountBoardMutationFrontierCommit(ctx,
				query.StoreCountBoardMutationFrontierCommitParams{
					BoardID: boardID.String(), WriterUid: tip.Writer.Bytes(),
					Sequence: int64(tip.Sequence), MarkerHash: tip.Hash.Bytes(),
				})
			if err != nil {
				return 0, fmt.Errorf("read source observed frontier: %w", err)
			}
			if found != 1 {
				return 0, errors.New("source observed frontier references missing history")
			}
		}
		if err := cloneMutation(ctx, reader, writer, marker); err != nil {
			return 0, err
		}
		tips[writerUID] = history.Tip{
			Writer: writerUID, Sequence: header.Sequence, Hash: marker.Hash(),
		}
		afterWriter, afterSequence, previous = row.WriterUid, row.Sequence, marker.Hash()
		count++
	}
	if count == 0 {
		return 0, errors.New("source board has no retained mutation history")
	}
	rows, err := reader.BoardHistoryListTips(ctx, boardID.String())
	if err != nil {
		return 0, fmt.Errorf("read source writer tips: %w", err)
	}
	for _, row := range rows {
		uid, err := board.ParseWriterUID(row.WriterUid)
		if err != nil {
			return 0, err
		}
		digest, err := history.ParseDigest(row.MarkerHash)
		if err != nil {
			return 0, err
		}
		tip, found := tips[uid]
		if !found || tip.Sequence != uint64(row.Sequence) || tip.Hash != digest {
			return 0, errors.New("source writer tip does not identify its final mutation")
		}
		if err := writer.BoardHistoryInsertTip(ctx, query.BoardHistoryInsertTipParams{
			BoardID: boardID.String(), WriterUid: row.WriterUid,
			Sequence: row.Sequence, MarkerHash: row.MarkerHash,
		}); err != nil {
			return 0, fmt.Errorf("retain cloned writer tip: %w", err)
		}
		delete(tips, uid)
	}
	if len(tips) != 0 {
		return 0, errors.New("source writer has mutations but no retained tip")
	}
	return count, nil
}

// cloneMutation verifies the complete event stream while retaining its bounded
// parts. Marker and blob references follow only after full consumption proves
// payload digest and event count; the enclosing transaction hides partial work.
func cloneMutation(
	ctx context.Context,
	reader *query.Queries,
	writer *query.Queries,
	marker *history.Marker,
) error {
	header := marker.Header()
	blobs := make(map[string]int64)
	for retained, err := range history.MutationEvents(marker,
		func(index int64) ([]byte, error) {
			part, err := reader.BoardHistoryClonePart(ctx, query.BoardHistoryClonePartParams{
				BoardID: header.Board.String(), WriterUid: header.Writer.Bytes(),
				Sequence: int64(header.Sequence), PartIndex: index,
			})
			if err != nil {
				return nil, fmt.Errorf("read source mutation part: %w", err)
			}
			if err := writer.BoardHistoryInsertPart(ctx, query.BoardHistoryInsertPartParams{
				BoardID: header.Board.String(), WriterUid: header.Writer.Bytes(),
				Sequence: int64(header.Sequence), PartIndex: index, Bytes: part,
			}); err != nil {
				return nil, fmt.Errorf("retain mutation part: %w", err)
			}
			return part, nil
		}) {
		if err != nil {
			return fmt.Errorf("verify source mutation events: %w", err)
		}
		if value, ok := retained.Event.Payload.(*history.AttachmentAdd); ok {
			if size, found := blobs[value.Digest]; found && size != value.SizeBytes {
				return errors.New("source attachment digest has conflicting sizes")
			}
			blobs[value.Digest] = value.SizeBytes
		}
	}
	if err := writer.BoardHistoryInsertCommit(ctx, query.BoardHistoryInsertCommitParams{
		BoardID: header.Board.String(), WriterUid: header.Writer.Bytes(),
		Sequence: int64(header.Sequence), PreviousHash: header.Previous.Bytes(),
		MarkerHash: marker.Hash().Bytes(), MarkerBytes: marker.Bytes(),
		PartCount: marker.PartCount(), EventCount: int64(marker.EventCount()),
		PayloadHash: marker.PayloadHash().Bytes(),
	}); err != nil {
		return fmt.Errorf("retain mutation marker: %w", err)
	}
	for digest, size := range blobs {
		if err := writer.BoardHistoryInsertBlobReference(ctx, query.BoardHistoryInsertBlobReferenceParams{
			BoardID: header.Board.String(), WriterUid: header.Writer.Bytes(),
			Sequence: int64(header.Sequence), Digest: digest, SizeBytes: size,
		}); err != nil {
			return fmt.Errorf("retain mutation attachment reference: %w", err)
		}
	}
	return nil
}
