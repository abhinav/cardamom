package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownInput_Read(t *testing.T) {
	t.Run("Argument", func(t *testing.T) {
		input := MarkdownInput{
			Context:    t.Context(),
			Stdin:      strings.NewReader("stdin"),
			IsTerminal: true,
		}
		argument := "argument"

		got, provided, err := input.Read(&argument)
		require.NoError(t, err)
		assert.True(t, provided)
		assert.Equal(t, "argument", got)
	})

	t.Run("Dash", func(t *testing.T) {
		input := MarkdownInput{
			Context:    t.Context(),
			Stdin:      strings.NewReader("from stdin\n"),
			IsTerminal: true,
		}
		argument := "-"

		got, provided, err := input.Read(&argument)
		require.NoError(t, err)
		assert.True(t, provided)
		assert.Equal(t, "from stdin\n", got)
	})

	t.Run("Piped", func(t *testing.T) {
		input := MarkdownInput{
			Context:    t.Context(),
			Stdin:      strings.NewReader("piped\n"),
			IsTerminal: false,
		}

		got, provided, err := input.Read(nil)
		require.NoError(t, err)
		assert.True(t, provided)
		assert.Equal(t, "piped\n", got)
	})

	t.Run("OmittedInteractive", func(t *testing.T) {
		input := MarkdownInput{
			Context:    t.Context(),
			Stdin:      strings.NewReader("ignored"),
			IsTerminal: true,
		}

		got, provided, err := input.Read(nil)
		require.NoError(t, err)
		assert.False(t, provided)
		assert.Empty(t, got)
	})

	t.Run("ReadFailure", func(t *testing.T) {
		input := MarkdownInput{
			Context:    t.Context(),
			Stdin:      failingReader{},
			IsTerminal: false,
		}

		_, _, err := input.Read(nil)
		assert.EqualError(t, err, "read Markdown from standard input: read failed")
	})
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}
