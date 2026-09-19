// Package boardhistory encodes and verifies retained board mutation history.
// Repository packages own persistence and keep relational projections
// authoritative for current board state. Exported payload types describe
// semantic changes; private V1 records own the persisted JSON contract.
// Retained V1 bytes and meanings are immutable; representation changes require
// a new version.
//
// A mutation is an ordered stream of events followed by a terminal Marker.
// Its Header links to the previous mutation from the same writer and records
// the writer tips observed before this mutation as a causal frontier.
// Events have stable identities from their writer, mutation sequence, and
// ordinal within the stream; payload part boundaries do not identify events.
//
// EncodeMutation streams events into bounded parts and returns the marker.
// NewMutationEncoder supports repositories that produce events incrementally.
// The marker seals the complete payload but does not persist or commit it.
// Repositories must retain parts, marker, and writer tip in the same transaction
// as the corresponding projection changes. Encoding may fail after emitting
// parts; the caller must then discard or roll back those parts.
//
// DecodeMarker establishes the retained marker's canonical shape and internal
// consistency. MutationEvents decodes its payload and verifies the payload
// digest and event count when iteration completes without an error.
// Writes buffer bounded parts; reads buffer one complete event at a time.
// An early stop leaves that verification incomplete, even if every event seen
// so far was valid. The repository additionally checks that predecessor and
// frontier references resolve to retained history before accepting a mutation.
package boardhistory
