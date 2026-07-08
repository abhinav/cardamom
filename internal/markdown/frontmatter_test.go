package markdown_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/markdown"
)

func TestEncodeFrontmatter(t *testing.T) {
	metadata := struct {
		Cardamom struct {
			BoardID string `yaml:"board_id"`
		} `yaml:"cardamom"`
	}{}
	metadata.Cardamom.BoardID = "board-1"

	encoded, err := markdown.EncodeFrontmatter(metadata, []byte("# Body\n"))
	require.NoError(t, err)

	assert.Equal(t, "---\ncardamom:\n  board_id: board-1\n---\n\n# Body\n", string(encoded))
}

func TestDecodeFrontmatter(t *testing.T) {
	var metadata struct {
		Cardamom struct {
			BoardID string `yaml:"board_id"`
		} `yaml:"cardamom"`
	}

	body, found, err := markdown.DecodeFrontmatter([]byte("---\ncardamom:\n  board_id: board-1\n---\n\n# Body\n"), &metadata)
	require.NoError(t, err)

	assert.True(t, found)
	assert.Equal(t, "board-1", metadata.Cardamom.BoardID)
	assert.Equal(t, []byte("# Body\n"), body)
}

func TestDecodeFrontmatterAbsent(t *testing.T) {
	body, found, err := markdown.DecodeFrontmatter([]byte("# Body\n"), &struct{}{})
	require.NoError(t, err)

	assert.False(t, found)
	assert.Nil(t, body)
}

func TestDecodeFrontmatterReaderStreamsBody(t *testing.T) {
	var metadata struct {
		Cardamom struct {
			BoardID string `yaml:"board_id"`
		} `yaml:"cardamom"`
	}
	source := iotest.OneByteReader(strings.NewReader(
		"---\ncardamom:\n  board_id: board-1\n---\n\n# Body\n",
	))

	body, found, err := markdown.DecodeFrontmatterReader(source, &metadata)
	require.NoError(t, err)
	content, err := io.ReadAll(body)
	require.NoError(t, err)

	assert.True(t, found)
	assert.Equal(t, "board-1", metadata.Cardamom.BoardID)
	assert.Equal(t, []byte("# Body\n"), content)
}

func TestDecodeFrontmatterReaderLeavesBodyUnread(t *testing.T) {
	bodyErr := errors.New("body read failed")
	source := io.MultiReader(
		strings.NewReader("---\ncardamom:\n  board_id: board-1\n---\n\n"),
		iotest.ErrReader(bodyErr),
	)

	body, found, err := markdown.DecodeFrontmatterReader(source, &struct{}{})
	require.NoError(t, err)
	assert.True(t, found)

	_, err = io.ReadAll(body)
	assert.ErrorIs(t, err, bodyErr)
}
