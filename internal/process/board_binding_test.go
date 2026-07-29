package process

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckoutBoardBinding_rejectsMultipleLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".cardamom-board")
	require.NoError(t, os.WriteFile(path, []byte("board-one\nboard-two\n"), 0o600))

	_, err := (&checkoutBoardBinding{path: path}).Read()

	assert.ErrorContains(t, err, "must contain one board ID")
}
