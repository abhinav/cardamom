package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViewLineageIDPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lineage.db")
	first, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	firstView, err := first.View(t.Context())
	require.NoError(t, err)
	firstID, err := firstView.LineageID(t.Context())
	require.NoError(t, err)
	require.NoError(t, firstView.Done())
	require.NoError(t, first.Close())

	second, err := Open(t.Context(), Config{Path: path})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, second.Close()) })
	secondView, err := second.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, secondView.Done()) }()
	secondID, err := secondView.LineageID(t.Context())
	require.NoError(t, err)

	assert.Equal(t, firstID, secondID)
}
