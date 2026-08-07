package boardcopy

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strconv"
	"time"
)

// semanticDigest incrementally encodes the version 2 board-copy identity.
type semanticDigest struct {
	hash    hash.Hash
	encoder semanticEncoder
}

func newSemanticDigest() *semanticDigest {
	digest := sha256.New()
	return &semanticDigest{
		hash: digest, encoder: semanticEncoder{hash: digest},
	}
}

func (d *semanticDigest) add(record Record) {
	record.encodeSemantic(&d.encoder)
}

func (d *semanticDigest) sum() string {
	return "sha256:" + hex.EncodeToString(d.hash.Sum(nil))
}

type semanticEncoder struct {
	hash hash.Hash // required
}

func (e *semanticEncoder) marker(tag string) {
	e.field(tag, nil)
}

func (e *semanticEncoder) string(tag string, value string) {
	e.field(tag, []byte(value))
}

func (e *semanticEncoder) optionalString(tag string, value *string) {
	if value == nil {
		e.field(tag, []byte{0})
		return
	}
	e.field(tag, append([]byte{1}, []byte(*value)...))
}

func (e *semanticEncoder) integer(tag string, value int) {
	e.field(tag, strconv.AppendInt(nil, int64(value), 10))
}

func (e *semanticEncoder) signed(tag string, value int64) {
	e.field(tag, strconv.AppendInt(nil, value, 10))
}

func (e *semanticEncoder) unsigned(tag string, value uint64) {
	e.field(tag, strconv.AppendUint(nil, value, 10))
}

func (e *semanticEncoder) timestamp(tag string, value time.Time) {
	e.field(tag, []byte(value.UTC().Format(time.RFC3339Nano)))
}

func (e *semanticEncoder) optionalTimestamp(tag string, value *time.Time) {
	if value == nil {
		e.field(tag, []byte{0})
		return
	}
	text := value.UTC().Format(time.RFC3339Nano)
	e.field(tag, append([]byte{1}, []byte(text)...))
}

func (e *semanticEncoder) field(tag string, value []byte) {
	// Tags and values are framed independently to prevent concatenation aliases.
	e.writePart([]byte(tag))
	e.writePart(value)
}

func (e *semanticEncoder) writePart(value []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = e.hash.Write(size[:])
	_, _ = e.hash.Write(value)
}
