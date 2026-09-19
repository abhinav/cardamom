package issue

import (
	"errors"
	"fmt"
	"io"
)

const uidSize = 16

// UID is an issue's immutable private identity.
//
// Public issue IDs remain the selectors accepted by callers. UID identifies
// the issue and its persisted relationships independently of that public ID.
type UID struct {
	value [uidSize]byte
}

// GenerateUID reads one UID from source.
// It rejects short input and the all-zero value.
func GenerateUID(source io.Reader) (UID, error) {
	var value [uidSize]byte
	if _, err := io.ReadFull(source, value[:]); err != nil {
		return UID{}, fmt.Errorf("read issue UID entropy: %w", err)
	}
	return ParseUID(value[:])
}

// ParseUID copies one non-zero 16-byte persisted UID.
func ParseUID(value []byte) (UID, error) {
	if len(value) != uidSize {
		return UID{}, fmt.Errorf("issue UID must contain %d bytes", uidSize)
	}
	var uid UID
	copy(uid.value[:], value)
	if uid == (UID{}) {
		return UID{}, errors.New("issue UID must not be zero")
	}
	return uid, nil
}

// Bytes returns a copy of the UID's persistence representation.
func (u UID) Bytes() []byte {
	return append([]byte(nil), u.value[:]...)
}
