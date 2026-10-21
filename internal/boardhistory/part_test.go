package boardhistory

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartWriter_WriteString_streamsBoundedParts(t *testing.T) {
	value := strings.Repeat("segment", mutationPartLimit)
	var parts []Part
	writer := newPartWriter(func(part Part) error {
		parts = append(parts, part)
		return nil
	})

	written, err := io.WriteString(writer, value)
	require.NoError(t, err)
	assert.Equal(t, len(value), written)
	assert.NotEmpty(t, parts)
	for _, part := range parts {
		assert.Len(t, part.Bytes, mutationPartLimit)
	}
	partCount, _, err := writer.finish()
	require.NoError(t, err)
	assert.Equal(t, int64(len(parts)), partCount)

	var encoded strings.Builder
	for _, part := range parts {
		_, _ = encoded.Write(part.Bytes)
	}
	assert.Equal(t, value, encoded.String())
}
