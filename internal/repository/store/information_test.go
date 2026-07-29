package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestView_ReadInformationReportsSchemaAndRevision(t *testing.T) {
	persistence, err := Open(t.Context(), Config{
		Path: filepath.Join(t.TempDir(), "information.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, change.PublishRevision(t.Context(), reservation))
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	information, err := view.ReadInformation(t.Context())
	require.NoError(t, err)
	assert.Equal(t, Information{
		DatabaseSchemaVersion: SchemaVersion(),
		CodeSchemaVersion:     SchemaVersion(),
		Revision:              1,
	}, information)
}
