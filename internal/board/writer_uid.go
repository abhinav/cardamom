package board

import (
	"errors"
	"fmt"
	"io"
)

const writerUIDSize = 16

// WriterUID is the immutable private identity of one stored board replica.
// A synchronization clone preserves the board ID but assigns a new WriterUID,
// while an independent copy assigns both identities anew.
type WriterUID struct {
	value [writerUIDSize]byte
}

// GenerateWriterUID reads one writer UID from source.
// It rejects short input and the all-zero value.
func GenerateWriterUID(source io.Reader) (WriterUID, error) {
	var value [writerUIDSize]byte
	if _, err := io.ReadFull(source, value[:]); err != nil {
		return WriterUID{}, fmt.Errorf("read board writer UID entropy: %w", err)
	}
	return ParseWriterUID(value[:])
}

// ParseWriterUID copies one non-zero 16-byte persisted writer UID.
func ParseWriterUID(value []byte) (WriterUID, error) {
	if len(value) != writerUIDSize {
		return WriterUID{}, fmt.Errorf(
			"board writer UID must contain %d bytes",
			writerUIDSize,
		)
	}
	var uid WriterUID
	copy(uid.value[:], value)
	if uid == (WriterUID{}) {
		return WriterUID{}, errors.New("board writer UID must not be zero")
	}
	return uid, nil
}

// Bytes returns a copy of the UID's persistence representation.
func (u WriterUID) Bytes() []byte {
	return append([]byte(nil), u.value[:]...)
}
