package boardhistory

import (
	"errors"
	"fmt"
	"iter"
	"math"
)

// EncodeMutation validates events and streams their bytes to emit. Parts may
// reach emit before a later event or emit call fails. The returned marker is
// completion record for the encoded mutation; it does not commit storage.
// Callers must discard or roll back emitted parts on error.
func EncodeMutation(
	header Header,
	events iter.Seq2[Event, error],
	emit func(Part) error,
) (*Marker, error) {
	if events == nil {
		return nil, errors.New("mutation events and part sink required")
	}
	encoder, err := NewMutationEncoder(header, emit)
	if err != nil {
		return nil, err
	}
	for event, eventErr := range events {
		if eventErr != nil {
			return nil, eventErr
		}
		if err := encoder.AppendEvent(event); err != nil {
			return nil, err
		}
	}
	return encoder.Finish()
}

// MutationEncoder incrementally writes one mutation's canonical events into
// bounded payload parts. It retains the first failure that AppendEvent
// encounters while encoding an event or emitting a part. Finish returns the
// terminal marker only after at least one event and no retained failure.
type MutationEncoder struct {
	header   Header
	parts    *partWriter
	count    uint64
	err      error
	finished bool
}

// NewMutationEncoder validates a mutation header and constructs its bounded
// event encoder. The part sink may write into the transaction that owns the
// related projection changes.
func NewMutationEncoder(header Header, emit func(Part) error) (*MutationEncoder, error) {
	header = canonicalHeader(header)
	if err := validateHeader(header); err != nil {
		return nil, err
	}
	if emit == nil {
		return nil, errors.New("mutation part sink required")
	}
	return &MutationEncoder{header: header, parts: newPartWriter(emit)}, nil
}

// AppendEvent validates and writes one event while retaining at most one
// bounded payload part in memory. Before Finish, later calls return the first
// failure encountered while encoding an event or emitting a part.
func (me *MutationEncoder) AppendEvent(event Event) error {
	if me.finished {
		return errors.New("mutation event encoder already finished")
	}
	if me.err != nil {
		return me.err
	}
	if me.count == math.MaxInt64 {
		me.err = errors.New("mutation event count exceeds persistence range")
		return me.err
	}
	if err := writeEvent(me.parts, event); err != nil {
		me.err = fmt.Errorf("encode mutation event %d: %w", me.count, err)
		return me.err
	}
	if _, err := me.parts.Write([]byte{'\n'}); err != nil {
		me.err = fmt.Errorf("terminate mutation event %d: %w", me.count, err)
		return me.err
	}
	me.count++
	return nil
}

// Finish flushes the final payload part and returns its terminal marker. A final
// flush failure is returned directly and is not retained because the encoder
// is already finished. Parts emitted before an error are not committed and
// remain the caller's concern.
func (me *MutationEncoder) Finish() (*Marker, error) {
	if me.finished {
		return nil, errors.New("mutation event encoder already finished")
	}
	me.finished = true
	if me.err != nil {
		return nil, me.err
	}
	if me.count == 0 {
		return nil, errors.New("mutation event stream is empty")
	}
	partCount, payloadHash, err := me.parts.finish()
	if err != nil {
		return nil, fmt.Errorf("finish mutation payload: %w", err)
	}
	return newMarker(me.header, partCount, me.count, payloadHash)
}
