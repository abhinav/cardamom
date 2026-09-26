package board

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateWriterUID(t *testing.T) {
	value := bytes.Repeat([]byte{0x42}, writerUIDSize)

	uid, err := GenerateWriterUID(bytes.NewReader(value))
	require.NoError(t, err)

	assert.Equal(t, value, uid.Bytes())
	encoded := uid.Bytes()
	encoded[0] = 0
	assert.Equal(t, value, uid.Bytes())
}

func TestGenerateWriterUID_rejectsInvalidEntropy(t *testing.T) {
	t.Run("Short", func(t *testing.T) {
		_, err := GenerateWriterUID(bytes.NewReader(make([]byte, writerUIDSize-1)))

		assert.ErrorContains(t, err, "read board writer UID entropy")
	})

	t.Run("Zero", func(t *testing.T) {
		_, err := GenerateWriterUID(bytes.NewReader(make([]byte, writerUIDSize)))

		assert.ErrorContains(t, err, "board writer UID must not be zero")
	})
}

func TestParseWriterUID_rejectsInvalidValues(t *testing.T) {
	t.Run("Length", func(t *testing.T) {
		_, err := ParseWriterUID(make([]byte, writerUIDSize-1))

		assert.ErrorContains(t, err, "board writer UID must contain 16 bytes")
	})

	t.Run("Zero", func(t *testing.T) {
		_, err := ParseWriterUID(make([]byte, writerUIDSize))

		assert.ErrorContains(t, err, "board writer UID must not be zero")
	})
}
