package boardhistory

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"slices"

	"go.abhg.dev/cardamom/internal/board"
)

// Header identifies a mutation and the history its writer observed.
// Sequence and Previous link the writer's own mutations; Frontier captures
// causal knowledge across writers. Timestamps do not establish this ordering.
// Encoders copy and sort Frontier, then reject duplicate writers or a missing
// own predecessor. DecodeMarker requires the retained order to be canonical.
type Header struct {
	// Board identifies the logical board shared by future replicas.
	Board board.ID // required

	// Writer identifies the replica that produced this mutation.
	Writer board.WriterUID // required

	// Sequence is the one-based position in the writer's chain, at most 2^63-1.
	Sequence uint64 // required

	// Previous is zero for sequence one and the preceding marker digest later.
	Previous Digest

	// Frontier lists every writer tip observed before this mutation.
	// For Sequence > 1 it must include this writer at Sequence-1 with Previous.
	// Sequence 1 must not include this writer. A nil frontier means no observed tips.
	Frontier []Tip
}

// Tip identifies one observed writer chain head.
type Tip struct {
	// Writer identifies the observed replica.
	Writer board.WriterUID // required

	// Sequence identifies the observed writer mutation and is between 1 and 2^63-1.
	Sequence uint64 // required

	// Hash is the nonzero digest of the observed mutation marker.
	Hash Digest // required
}

func validateHeader(header Header) error {
	if err := validateBoardIDV1(header.Board.String()); err != nil {
		return fmt.Errorf("invalid mutation board identity: %w", err)
	}
	if _, err := board.ParseWriterUID(header.Writer.Bytes()); err != nil {
		return fmt.Errorf("invalid mutation writer identity: %w", err)
	}
	if header.Sequence == 0 || header.Sequence > math.MaxInt64 {
		return errors.New("mutation sequence is outside the persistence range")
	}
	if header.Sequence == 1 && header.Previous != (Digest{}) {
		return errors.New("first mutation must not name a predecessor")
	}
	if header.Sequence > 1 && header.Previous == (Digest{}) {
		return errors.New("later mutation requires a predecessor")
	}
	var previousWriter []byte
	var ownTip *Tip
	for index := range header.Frontier {
		tip := &header.Frontier[index]
		writerBytes := tip.Writer.Bytes()
		if _, err := board.ParseWriterUID(writerBytes); err != nil {
			return fmt.Errorf("invalid frontier writer at index %d: %w", index, err)
		}
		if tip.Sequence == 0 || tip.Sequence > math.MaxInt64 || tip.Hash == (Digest{}) {
			return fmt.Errorf("invalid frontier tip at index %d", index)
		}
		if index > 0 && bytes.Compare(previousWriter, writerBytes) >= 0 {
			return errors.New("mutation frontier must be sorted by unique writer identity")
		}
		previousWriter = writerBytes
		if tip.Writer == header.Writer {
			ownTip = tip
		}
	}
	if header.Sequence == 1 && ownTip != nil {
		return errors.New("first writer mutation must not observe its own tip")
	}
	if header.Sequence > 1 &&
		(ownTip == nil || ownTip.Sequence != header.Sequence-1 || ownTip.Hash != header.Previous) {
		return errors.New("later mutation requires its predecessor in the observed frontier")
	}
	return nil
}

func canonicalHeader(header Header) Header {
	header.Frontier = slices.Clone(header.Frontier)
	slices.SortFunc(header.Frontier, func(left, right Tip) int {
		return bytes.Compare(left.Writer.Bytes(), right.Writer.Bytes())
	})
	return header
}
