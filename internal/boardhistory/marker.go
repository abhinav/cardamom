package boardhistory

import (
	"bytes"
	"crypto/sha256"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"go.abhg.dev/cardamom/internal/board"
)

const mutationMarkerLimit = 64 * 1024

// Marker records a mutation's payload digest, part and event counts, and Header.
// Its hash identifies the mutation in the writer chain and in causal frontiers.
// DecodeMarker and MutationEncoder.Finish are its supported construction paths;
// the zero value is invalid. Construction establishes canonical shape and
// internal consistency, not authenticity or durable storage. The repository
// must still verify the payload and resolve references to retained history.
type Marker struct {
	header      Header
	partCount   int64
	eventCount  uint64
	payloadHash Digest
	hash        Digest
	bytes       []byte
}

// DecodeMarker copies and parses one canonical V1 marker, rejecting malformed
// identities, counts, and causal references. It does not read payload parts or
// establish that the predecessor and frontier tips exist in retained history.
func DecodeMarker(encoded []byte) (*Marker, error) {
	if len(encoded) == 0 || len(encoded) > mutationMarkerLimit {
		return nil, fmt.Errorf(
			"mutation marker size must be between 1 and %d bytes",
			mutationMarkerLimit,
		)
	}
	version, err := decodeVersion(encoded, "mutation marker")
	if err != nil {
		return nil, err
	}
	if version != 1 {
		return nil, fmt.Errorf("unsupported mutation marker version %d", version)
	}
	var wire markerV1
	if err := json.Unmarshal(encoded, &wire, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode mutation marker: %w", err)
	}
	if wire.Frontier == nil {
		return nil, errors.New("mutation marker requires a frontier array")
	}
	canonical, err := encodeMarkerV1(wire)
	if err != nil {
		return nil, fmt.Errorf("canonicalize mutation marker: %w", err)
	}
	if !bytes.Equal(canonical, encoded) {
		return nil, errors.New("mutation marker is not canonical")
	}
	if err := validateBoardIDV1(wire.BoardID); err != nil {
		return nil, fmt.Errorf("decode mutation board identity: %w", err)
	}
	boardID := board.ID(wire.BoardID)
	writer, err := board.ParseWriterUID(wire.WriterUID)
	if err != nil {
		return nil, fmt.Errorf("decode mutation writer identity: %w", err)
	}
	previous, err := ParseDigest(wire.Previous)
	if err != nil {
		return nil, fmt.Errorf("decode mutation predecessor: %w", err)
	}
	payloadHash, err := ParseDigest(wire.PayloadHash)
	if err != nil {
		return nil, fmt.Errorf("decode mutation payload digest: %w", err)
	}
	if payloadHash == (Digest{}) {
		return nil, errors.New("mutation payload digest must not be zero")
	}
	frontier := make([]Tip, 0, len(wire.Frontier))
	for index, wireTip := range wire.Frontier {
		writer, err := board.ParseWriterUID(wireTip.WriterUID)
		if err != nil {
			return nil, fmt.Errorf("decode mutation frontier tip %d writer: %w", index, err)
		}
		digest, err := ParseDigest(wireTip.Hash)
		if err != nil {
			return nil, fmt.Errorf("decode mutation frontier tip %d digest: %w", index, err)
		}
		frontier = append(frontier, Tip{
			Writer: writer, Sequence: wireTip.Sequence, Hash: digest,
		})
	}
	header := Header{
		Board: boardID, Writer: writer, Sequence: wire.Sequence,
		Previous: previous, Frontier: frontier,
	}
	if err := validateHeader(header); err != nil {
		return nil, err
	}
	if wire.PartCount <= 0 || wire.EventCount == 0 {
		return nil, errors.New("mutation marker requires parts and events")
	}
	return &Marker{
		header: header, partCount: wire.PartCount,
		eventCount: wire.EventCount, payloadHash: payloadHash,
		hash: Digest(sha256.Sum256(encoded)), bytes: append([]byte(nil), encoded...),
	}, nil
}

// newMarker seals the validated header and completed payload owned by an encoder.
func newMarker(
	header Header,
	partCount int64,
	eventCount uint64,
	payloadHash Digest,
) (*Marker, error) {
	wire := markerV1{
		Version: 1, BoardID: header.Board.String(),
		WriterUID: header.Writer.Bytes(), Sequence: header.Sequence,
		Previous: header.Previous.Bytes(), PartCount: partCount,
		EventCount: eventCount, PayloadHash: payloadHash.Bytes(),
		Frontier: make([]tipV1, 0, len(header.Frontier)),
	}
	for _, tip := range header.Frontier {
		wire.Frontier = append(wire.Frontier, tipV1{
			WriterUID: tip.Writer.Bytes(), Sequence: tip.Sequence,
			Hash: tip.Hash.Bytes(),
		})
	}
	encoded, err := encodeMarkerV1(wire)
	if err != nil {
		return nil, fmt.Errorf("encode mutation marker: %w", err)
	}
	if len(encoded) > mutationMarkerLimit {
		return nil, fmt.Errorf(
			"mutation marker exceeds %d bytes",
			mutationMarkerLimit,
		)
	}
	// V1 canonicalization represents JSON numbers as IEEE-754 doubles and can
	// round large integers. Reconstruct from the retained bytes so exposed
	// counts and frontier sequences agree with the marker's hashed content;
	// validation also rejects any predecessor relation broken by rounding.
	marker, err := DecodeMarker(encoded)
	if err != nil {
		return nil, fmt.Errorf("validate mutation marker: %w", err)
	}
	return marker, nil
}

// Header returns the mutation identity retained by the marker.
func (m *Marker) Header() Header {
	header := m.header
	header.Frontier = slices.Clone(header.Frontier)
	return header
}

// PartCount returns the number of payload parts preceding the marker.
func (m *Marker) PartCount() int64 { return m.partCount }

// EventCount returns the number of semantic events in the payload.
func (m *Marker) EventCount() uint64 { return m.eventCount }

// PayloadHash returns the SHA-256 digest of payload bytes in part-index order.
// Part boundaries do not contribute to this digest.
func (m *Marker) PayloadHash() Digest { return m.payloadHash }

// Hash returns the SHA-256 digest of the canonical marker bytes.
// It covers Header, PartCount, EventCount, and PayloadHash.
func (m *Marker) Hash() Digest { return m.hash }

// Bytes returns a copy of the canonical marker encoding.
func (m *Marker) Bytes() []byte { return append([]byte(nil), m.bytes...) }

// Digest is one SHA-256 mutation or payload digest.
type Digest [sha256.Size]byte

// ParseDigest copies a persisted SHA-256 digest.
func ParseDigest(value []byte) (Digest, error) {
	if len(value) != sha256.Size {
		return Digest{}, fmt.Errorf("mutation digest must contain %d bytes", sha256.Size)
	}
	var digest Digest
	copy(digest[:], value)
	return digest, nil
}

// Bytes returns a copy of the digest persistence representation.
func (d Digest) Bytes() []byte { return append([]byte(nil), d[:]...) }

// markerV1 is the strict retained representation decoded after V1 dispatch.
type markerV1 struct {
	Version     uint8   `json:"version"`
	BoardID     string  `json:"board_id"`
	WriterUID   []byte  `json:"writer_uid"`
	Sequence    uint64  `json:"sequence"`
	Previous    []byte  `json:"previous"`
	Frontier    []tipV1 `json:"frontier"`
	PartCount   int64   `json:"part_count"`
	EventCount  uint64  `json:"event_count"`
	PayloadHash []byte  `json:"payload_hash"`
}

// tipV1 is one frontier entry embedded in markerV1.
type tipV1 struct {
	WriterUID []byte `json:"writer_uid"`
	Sequence  uint64 `json:"sequence"`
	Hash      []byte `json:"hash"`
}

func encodeMarkerV1(marker markerV1) ([]byte, error) {
	encoded, err := json.Marshal(marker)
	if err != nil {
		return nil, err
	}
	canonical := jsontext.Value(encoded)
	if err := canonical.Canonicalize(); err != nil {
		return nil, err
	}
	return canonical, nil
}
