package boardhistory

import (
	"time"

	"go.abhg.dev/cardamom/internal/board"
)

// Event pairs a semantic change with its effective time.
// Payload and its nested pointers remain caller-owned and must stay unchanged
// until encoding returns. Copying Event does not copy the payload.
type Event struct {
	// OccurredAt records when the change took effect. Baseline events use the
	// observation time when the original change time is unavailable.
	OccurredAt time.Time // required

	// Payload contains one supported V1 semantic value.
	Payload Payload // required
}

// Kind returns the finite semantic kind of the event payload.
func (e Event) Kind() EventKind {
	if e.Payload == nil {
		return ""
	}
	return e.Payload.kind()
}

// Payload is one typed semantic value in the closed V1 event vocabulary.
// Implementations are non-nil pointers to the payload types in this package.
type Payload interface {
	kind() EventKind
	validateV1() error
	valueV1() any
}

// EventKind identifies one semantic event representation in format V1.
// Its string value is retained in history and is part of the format contract.
type EventKind string

// EventID identifies one event by writer mutation and retained ordinal.
type EventID struct {
	// Writer identifies the replica that produced the event.
	Writer board.WriterUID

	// Sequence identifies the mutation in the writer chain.
	Sequence uint64

	// Ordinal is the event's zero-based position within the mutation.
	Ordinal uint64
}
