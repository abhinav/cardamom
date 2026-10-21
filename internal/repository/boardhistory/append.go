package boardhistory

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"

	"go.abhg.dev/cardamom/internal/board"
	history "go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
)

// AppendV1 streams one non-empty semantic mutation onto the board's local
// writer chain. The caller must provide the transaction that owns the related
// projection writes and revision publication.
func AppendV1(
	ctx context.Context,
	db query.DBTX,
	boardID board.ID,
	events iter.Seq2[history.Event, error],
) error {
	if events == nil {
		return errors.New("append board history: event stream required")
	}
	appender := NewAppender(ctx, db, boardID)
	for event, err := range events {
		if err != nil {
			return err
		}
		if err := appender.Append(event); err != nil {
			return err
		}
	}
	return appender.Finish()
}

// Appender incrementally retains one mutation through a caller-owned
// transaction. It reads the current frontier lazily on the first event.
type Appender struct {
	ctx     context.Context
	queries *query.Queries
	boardID board.ID

	writer            board.WriterUID
	localTip          history.Tip
	encoder           *history.MutationEncoder
	retainedBlobSizes map[string]int64
	err               error
	finished          bool
}

// NewAppender constructs a lazy transaction-scoped history appender.
func NewAppender(ctx context.Context, db query.DBTX, boardID board.ID) *Appender {
	return &Appender{ctx: ctx, queries: query.New(db), boardID: boardID}
}

// HasEvents reports whether an event has entered this mutation.
func (a *Appender) HasEvents() bool { return a.encoder != nil }

// Append validates and incrementally retains one semantic event.
func (a *Appender) Append(event history.Event) error {
	if a.finished {
		return errors.New("append board history: mutation already finished")
	}
	if a.err != nil {
		return a.err
	}
	if a.encoder == nil {
		a.err = a.initialize()
		if a.err != nil {
			return a.err
		}
	}
	a.err = a.encoder.AppendEvent(event)
	if a.err == nil {
		a.err = a.retainEventBlob(event)
	}
	return a.err
}

func (a *Appender) retainEventBlob(event history.Event) error {
	var digest string
	var sizeBytes int64
	switch value := event.Payload.(type) {
	case *history.AttachmentAdd:
		digest, sizeBytes = value.Digest, value.SizeBytes
	default:
		return nil
	}
	if a.retainedBlobSizes == nil {
		a.retainedBlobSizes = make(map[string]int64)
	}
	if retainedSize, found := a.retainedBlobSizes[digest]; found && retainedSize != sizeBytes {
		return errors.New("append board history: attachment digest has conflicting sizes")
	}
	a.retainedBlobSizes[digest] = sizeBytes
	return nil
}

func (a *Appender) initialize() error {
	writerBytes, err := a.queries.BoardHistoryGetWriterUID(
		a.ctx, a.boardID.String(),
	)
	if err != nil {
		return fmt.Errorf("append board history: read writer identity: %w", err)
	}
	a.writer, err = board.ParseWriterUID(writerBytes)
	if err != nil {
		return fmt.Errorf("append board history: parse writer identity: %w", err)
	}
	tipRows, err := a.queries.BoardHistoryListTips(a.ctx, a.boardID.String())
	if err != nil {
		return fmt.Errorf("append board history: read frontier: %w", err)
	}
	frontier := make([]history.Tip, 0, len(tipRows))
	for _, row := range tipRows {
		tipWriter, err := board.ParseWriterUID(row.WriterUid)
		if err != nil {
			return fmt.Errorf("append board history: parse frontier writer: %w", err)
		}
		tipHash, err := history.ParseDigest(row.MarkerHash)
		if err != nil {
			return fmt.Errorf("append board history: parse frontier digest: %w", err)
		}
		tip := history.Tip{
			Writer: tipWriter, Sequence: uint64(row.Sequence), Hash: tipHash,
		}
		frontier = append(frontier, tip)
		if tipWriter == a.writer {
			a.localTip = tip
		}
	}
	if a.localTip.Writer != a.writer {
		return errors.New("append board history: local writer has no retained tip")
	}
	sequence := a.localTip.Sequence + 1
	a.encoder, err = history.NewMutationEncoder(
		history.Header{
			Board: a.boardID, Writer: a.writer, Sequence: sequence,
			Previous: a.localTip.Hash, Frontier: frontier,
		},
		func(part history.Part) error {
			return a.queries.BoardHistoryInsertPart(
				a.ctx,
				query.BoardHistoryInsertPartParams{
					BoardID: a.boardID.String(), WriterUid: a.writer.Bytes(),
					Sequence: int64(sequence), PartIndex: part.Index, Bytes: part.Bytes,
				},
			)
		},
	)
	if err != nil {
		return fmt.Errorf("append board history: initialize encoder: %w", err)
	}
	return nil
}

// Finish flushes the final bounded part, inserts the terminal marker and its
// blob references, and advances the local writer tip with a compare-and-update.
func (a *Appender) Finish() error {
	if a.finished {
		return errors.New("append board history: mutation already finished")
	}
	a.finished = true
	if a.err != nil {
		return a.err
	}
	if a.encoder == nil {
		return errors.New("append board history: mutation event stream is empty")
	}
	marker, err := a.encoder.Finish()
	if err != nil {
		return fmt.Errorf("append board history: finish events: %w", err)
	}
	sequence := a.localTip.Sequence + 1
	if err := a.queries.BoardHistoryInsertCommit(
		a.ctx,
		query.BoardHistoryInsertCommitParams{
			BoardID: a.boardID.String(), WriterUid: a.writer.Bytes(),
			Sequence: int64(sequence), PreviousHash: a.localTip.Hash.Bytes(),
			MarkerHash: marker.Hash().Bytes(), MarkerBytes: marker.Bytes(),
			PartCount: marker.PartCount(), EventCount: int64(marker.EventCount()),
			PayloadHash: marker.PayloadHash().Bytes(),
		},
	); err != nil {
		return fmt.Errorf("append board history: insert terminal marker: %w", err)
	}
	for _, digest := range slices.Sorted(maps.Keys(a.retainedBlobSizes)) {
		if err := a.queries.BoardHistoryInsertBlobReference(
			a.ctx,
			query.BoardHistoryInsertBlobReferenceParams{
				BoardID: a.boardID.String(), WriterUid: a.writer.Bytes(),
				Sequence: int64(sequence), Digest: digest,
				SizeBytes: a.retainedBlobSizes[digest],
			},
		); err != nil {
			return fmt.Errorf("append board history: retain attachment blob: %w", err)
		}
	}
	changed, err := a.queries.BoardHistoryAdvanceTip(
		a.ctx,
		query.BoardHistoryAdvanceTipParams{
			Sequence: int64(sequence), MarkerHash: marker.Hash().Bytes(),
			BoardID: a.boardID.String(), WriterUid: a.writer.Bytes(),
			PreviousSequence: int64(a.localTip.Sequence),
			PreviousHash:     a.localTip.Hash.Bytes(),
		},
	)
	if err != nil {
		return fmt.Errorf("append board history: advance writer tip: %w", err)
	}
	if changed != 1 {
		return errors.New("append board history: local writer tip changed")
	}
	return nil
}
