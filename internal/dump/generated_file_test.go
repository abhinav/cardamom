package dump

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFile_OpenLimitsContentAndTransfersCloseOwnership(t *testing.T) {
	source := &closeRecorder{Reader: bytes.NewReader([]byte("abcdef"))}
	file, err := NewGeneratedFile(GeneratedFileConfig{
		Path: "issues/an-a.md", Identity: "issue:an-a", Size: 3,
		Open: func() (io.ReadCloser, error) { return source, nil },
	})
	require.NoError(t, err)

	reader, err := file.Open()
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, []byte("abc"), content)
	assert.False(t, source.closed)

	require.NoError(t, reader.Close())
	assert.True(t, source.closed)
}

type closeRecorder struct {
	*bytes.Reader
	closed bool
}

func (r *closeRecorder) Close() error {
	r.closed = true
	return nil
}
