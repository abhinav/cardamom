package boardhistory

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"iter"
)

// RetainedEvent pairs a decoded event with its position in the writer chain.
// Its presence alone does not establish integrity of the complete mutation;
// consume MutationEvents to completion before accepting any results.
type RetainedEvent struct {
	// ID identifies the mutation and the event's zero-based ordinal within it.
	ID EventID

	// Event contains the decoded time and semantic payload, owned by the caller.
	Event Event
}

// MutationEvents reads canonical events in retained order.
// readPart supplies parts by zero-based index; it must keep each returned slice
// stable until the next call. Each traversal starts at part zero, so repeated
// traversal requires readPart to support rereading the same retained parts.
// Decoding buffers one complete event and its canonical encoding, so memory
// grows with the largest event rather than only with the bounded part size.
//
// An error is yielded once and terminates the sequence. Events yielded before
// that error are only a prefix: event count and payload digest are verified only
// when the sequence is consumed to completion without an error. Breaking early
// stops reading immediately and does not establish the mutation's integrity.
func MutationEvents(
	marker *Marker,
	readPart func(int64) ([]byte, error),
) iter.Seq2[RetainedEvent, error] {
	return func(yield func(RetainedEvent, error) bool) {
		payload, err := newPayloadReader(marker, readPart)
		if err != nil {
			yield(RetainedEvent{}, err)
			return
		}
		decoder := jsontext.NewDecoder(payload)
		var count uint64
		for {
			encoded, err := decoder.ReadValue()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				yield(RetainedEvent{}, fmt.Errorf("read mutation event %d: %w", count, err))
				return
			}
			event, err := decodeEvent(encoded)
			if err != nil {
				yield(RetainedEvent{}, fmt.Errorf("decode mutation event %d: %w", count, err))
				return
			}
			retained := RetainedEvent{
				ID: EventID{
					Writer:   marker.header.Writer,
					Sequence: marker.header.Sequence,
					Ordinal:  count,
				},
				Event: event,
			}
			if !yield(retained, nil) {
				return
			}
			count++
		}
		if count != marker.eventCount {
			yield(RetainedEvent{}, fmt.Errorf("mutation contains %d events; marker declares %d", count, marker.eventCount))
		}
	}
}
