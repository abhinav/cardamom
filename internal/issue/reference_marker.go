package issue

import (
	"context"
	"encoding/hex"
)

// MarkerReader reads retired public IDs independently of board selection.
// A marker may refer to issues on several boards, including absent boards.
type MarkerReader interface {
	// ReadMarker returns nil when id has not been retired.
	ReadMarker(context.Context, ID) (*ReferenceMarker, error)
}

// ReferenceMarker preserves an ambiguous public ID after a collision.
// Its candidates identify all known issues that used the ID; the marker
// itself is not an issue and cannot be edited, claimed, or used as an edge.
type ReferenceMarker struct {
	// ID is permanently reserved for this marker.
	ID ID // required

	// Candidates contains distinct immutable identities in UID order.
	// Candidate membership only grows as more collision evidence arrives.
	Candidates []MarkerCandidate // required
}

// MarkerCandidate identifies one interpretation of a retired public ID.
type MarkerCandidate struct {
	// UID follows the issue even when its public ID changes again.
	UID UID // required

	// BoardID is the issue's logical board, whether or not it is present.
	BoardID string // required

	// ID is the current public ID when the issue is available locally,
	// or its saved deterministic replacement when unavailable.
	ID ID // required

	// Available reports whether the issue exists in this store.
	Available bool

	// Title is the current resolved title, when available.
	Title *string
}

// ReplacementID derives one participant's replacement without consulting an
// allocator or other participants. If that ID also collides, it becomes a
// marker and this operation is applied to that newly retired ID in turn.
// The full UID keeps distinct participants distinct without an arrival-order
// choice of suffix length.
func ReplacementID(retired ID, uid UID) ID {
	return ID(retired.String() + "-" + hex.EncodeToString(uid.Bytes()))
}
