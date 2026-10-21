package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	boardpkg "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type idStep struct {
	value string
	err   error
}

type sequenceIDs struct{ steps []idStep }

func (s *sequenceIDs) NewID(string) (string, error) {
	step := s.steps[0]
	s.steps = s.steps[1:]
	return step.value, step.err
}

func canonicalRevision(t *testing.T, persistence *store.Store) int64 {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	revision, err := view.CanonicalRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, view.Done())
	return revision
}

func assertBoardHistoryBaseline(
	t *testing.T,
	persistence *store.Store,
	boardID boardpkg.ID,
	wantEvents uint64,
) {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var markerBytes, writerBytes []byte
	require.NoError(t, view.QueryRowContext(t.Context(), `
SELECT mutation.marker_bytes, replica.writer_uid
FROM board_mutation_commits AS mutation
JOIN board_replica_identities AS replica ON replica.board_id = mutation.board_id
WHERE mutation.board_id = ? AND mutation.sequence = 1
`, boardID.String()).Scan(&markerBytes, &writerBytes))
	writer, err := boardpkg.ParseWriterUID(writerBytes)
	require.NoError(t, err)
	marker, err := boardhistory.DecodeMarker(markerBytes)
	require.NoError(t, err)
	assert.Equal(t, boardID, marker.Header().Board)
	assert.Equal(t, writer, marker.Header().Writer)
	assert.Equal(t, uint64(1), marker.Header().Sequence)
	assert.Empty(t, marker.Header().Frontier)
	assert.Equal(t, wantEvents, marker.EventCount())

	var visited uint64
	for _, err := range boardhistory.MutationEvents(
		marker,
		func(index int64) ([]byte, error) {
			var part []byte
			err := view.QueryRowContext(t.Context(), `
SELECT bytes
FROM board_mutation_parts
WHERE board_id = ? AND writer_uid = ? AND sequence = 1 AND part_index = ?
`, boardID.String(), writerBytes, index).Scan(&part)
			return part, err
		},
	) {
		require.NoError(t, err)
		visited++
	}
	assert.Equal(t, wantEvents, visited)
}

func initializeProjectCatalog(
	t *testing.T,
	persistence *store.Store,
	boardName *string,
	config Config,
) project.InitializedNamespace {
	t.Helper()
	namespace, err := initializeFreshProject(
		t.Context(),
		persistence,
		project.StoreInitializationRequest{
			ProjectName: "Project",
			BoardName:   boardName,
		},
		config,
	)
	require.NoError(t, err)
	return namespace
}

func insertProject(
	t *testing.T,
	persistence *store.Store,
	projectID string,
	name string,
) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at) VALUES (?, ?, 20)
	`, projectID, name)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}

func mustProjectID(t *testing.T, value string) project.ID {
	t.Helper()
	id, err := project.NewID(value)
	require.NoError(t, err)
	return id
}

func mustBoardID(t *testing.T, value string) boardpkg.ID {
	t.Helper()
	id, err := boardpkg.NewID(value)
	require.NoError(t, err)
	return id
}
