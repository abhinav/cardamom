package project

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryRetainsBoardSettingsLifecycleAndOverrides(t *testing.T) {
	persistence, err := store.Open(
		t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	require.NotNil(t, namespace.Board)
	repository := New(persistence, Config{
		Clock: fixedClock{now: time.Unix(30, 0).UTC()},
	})
	boardID := namespace.Board.ID()

	name := "Renamed board"
	description := "Shared description"
	_, err = repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID: boardID,
		Settings: board.SettingsEdit{
			Name: &name, Description: board.ReplaceDescription(&description),
		},
	})
	require.NoError(t, err)
	mutations := readProjectBoardHistory(t, persistence, boardID)
	require.Len(t, mutations, 2)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventBoardNameSet,
		boardhistory.EventBoardDescriptionSet,
	}, projectHistoryKinds(mutations[1]))

	_, err = repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID: boardID,
		Settings: board.SettingsEdit{
			Name: &name, Description: board.ReplaceDescription(&description),
		},
	})
	require.NoError(t, err)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 2)

	archived, err := repository.ArchiveBoard(
		t.Context(), board.NewInvocation("captain"),
		board.ArchiveRequest{BoardID: boardID},
	)
	require.NoError(t, err)
	assert.True(t, archived.Changed)
	mutations = readProjectBoardHistory(t, persistence, boardID)
	require.Len(t, mutations, 3)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventBoardArchiveSet,
	}, projectHistoryKinds(mutations[2]))

	archived, err = repository.ArchiveBoard(
		t.Context(), board.NewInvocation("captain"),
		board.ArchiveRequest{BoardID: boardID},
	)
	require.NoError(t, err)
	assert.False(t, archived.Changed)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 3)

	_, changed, err := repository.UnarchiveBoard(t.Context(), boardID)
	require.NoError(t, err)
	assert.True(t, changed)
	mutations = readProjectBoardHistory(t, persistence, boardID)
	require.Len(t, mutations, 4)
	assert.Equal(t, []boardhistory.EventKind{
		boardhistory.EventBoardArchiveSet,
	}, projectHistoryKinds(mutations[3]))

	_, changed, err = repository.UnarchiveBoard(t.Context(), boardID)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 4)

	projectPrefix, err := configuration.NewPrefix("project-")
	require.NoError(t, err)
	_, err = repository.UpdateProjectConfiguration(
		t.Context(),
		namespace.Project.ID(),
		configuration.Patch{
			Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
			Overrides: configuration.Overrides{Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &projectPrefix},
			}},
		},
	)
	require.NoError(t, err)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 4)

	prefix, err := configuration.NewPrefix("shared-")
	require.NoError(t, err)
	strategy, err := configuration.NewIDStrategy("random")
	require.NoError(t, err)
	summaryLimit, err := configuration.NewByteLimit(2048)
	require.NoError(t, err)
	attachmentLimit, err := configuration.NewByteLimit(4096)
	require.NoError(t, err)
	pinLimit, err := board.NewPinLimit(3)
	require.NoError(t, err)
	fields := []configuration.Field{
		configuration.FieldIssueIDPrefix,
		configuration.FieldIssueIDStrategy,
		configuration.FieldIssueSummaryMaxBytes,
		configuration.FieldAttachmentMaxBytes,
		configuration.FieldBoardPinsMaxCount,
	}
	patch := configuration.Patch{
		Fields: fields,
		Overrides: configuration.Overrides{
			Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{
					Prefix: &prefix, Strategy: &strategy,
				},
				Summary: configuration.SummaryOverrides{MaxBytes: &summaryLimit},
			},
			Attachment: configuration.AttachmentOverrides{MaxBytes: &attachmentLimit},
			Board: configuration.BoardOverrides{
				Pins: configuration.PinOverrides{MaxCount: &pinLimit},
			},
		},
	}
	_, err = repository.UpdateBoardConfiguration(t.Context(), boardID, patch)
	require.NoError(t, err)
	mutations = readProjectBoardHistory(t, persistence, boardID)
	require.Len(t, mutations, 5)
	configurationKinds := []boardhistory.EventKind{
		boardhistory.EventBoardIssueIDPrefixSet,
		boardhistory.EventBoardIssueIDStrategySet,
		boardhistory.EventBoardSummaryLimitSet,
		boardhistory.EventBoardAttachmentLimitSet,
		boardhistory.EventBoardPinLimitSet,
	}
	assert.Equal(t, configurationKinds, projectHistoryKinds(mutations[4]))

	_, err = repository.UpdateBoardConfiguration(t.Context(), boardID, patch)
	require.NoError(t, err)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 5)

	_, err = repository.UpdateBoardConfiguration(
		t.Context(), boardID, configuration.Patch{Fields: fields},
	)
	require.NoError(t, err)
	mutations = readProjectBoardHistory(t, persistence, boardID)
	require.Len(t, mutations, 6)
	assert.Equal(t, configurationKinds, projectHistoryKinds(mutations[5]))
	assertBoardConfigurationCleared(t, mutations[5])

	_, err = repository.UpdateBoardConfiguration(
		t.Context(), boardID, configuration.Patch{Fields: fields},
	)
	require.NoError(t, err)
	assert.Len(t, readProjectBoardHistory(t, persistence, boardID), 6)
}

func TestRepositoryRollsBackBoardSettingsWhenHistoryAppendFails(t *testing.T) {
	persistence, err := store.Open(
		t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	require.NotNil(t, namespace.Board)
	installProjectHistoryFailure(t, persistence)
	repository := New(persistence, Config{})
	name := "Must roll back"

	_, err = repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID:  namespace.Board.ID(),
		Settings: board.SettingsEdit{Name: &name},
	})
	require.ErrorContains(t, err, "history failure")
	stored, readErr := repository.Board(t.Context(), namespace.Board.ID())
	require.NoError(t, readErr)
	assert.Equal(t, namespace.Board.Name(), stored.Name())
	assert.Len(t, readProjectBoardHistory(t, persistence, namespace.Board.ID()), 1)
}

func TestRepositoryRollsBackBoardArchiveWhenHistoryAppendFails(t *testing.T) {
	persistence, err := store.Open(
		t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	require.NotNil(t, namespace.Board)
	installProjectHistoryFailure(t, persistence)
	repository := New(persistence, Config{})

	_, err = repository.ArchiveBoard(
		t.Context(), board.NewInvocation("captain"),
		board.ArchiveRequest{BoardID: namespace.Board.ID()},
	)
	require.ErrorContains(t, err, "history failure")
	stored, readErr := repository.Board(t.Context(), namespace.Board.ID())
	require.NoError(t, readErr)
	assert.Nil(t, stored.Archived())
	assert.Len(t, readProjectBoardHistory(t, persistence, namespace.Board.ID()), 1)
}

func TestRepositoryRollsBackBoardOverridesWhenHistoryAppendFails(t *testing.T) {
	persistence, err := store.Open(
		t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"},
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	require.NotNil(t, namespace.Board)
	installProjectHistoryFailure(t, persistence)
	repository := New(persistence, Config{})
	prefix, err := configuration.NewPrefix("rollback-")
	require.NoError(t, err)

	_, err = repository.UpdateBoardConfiguration(
		t.Context(),
		namespace.Board.ID(),
		configuration.Patch{
			Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
			Overrides: configuration.Overrides{Issue: configuration.IssueOverrides{
				ID: configuration.IssueIDOverrides{Prefix: &prefix},
			}},
		},
	)
	require.ErrorContains(t, err, "history failure")
	layers, readErr := repository.ReadConfigurationLayers(
		t.Context(), namespace.Board.ID(),
	)
	require.NoError(t, readErr)
	assert.Nil(t, layers.Board.Issue.ID.Prefix)
	assert.Len(t, readProjectBoardHistory(t, persistence, namespace.Board.ID()), 1)
}

func readProjectBoardHistory(
	t *testing.T,
	persistence *store.Store,
	boardID board.ID,
) [][]boardhistory.Event {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	rows, err := view.QueryContext(t.Context(), `
SELECT marker_bytes
FROM board_mutation_commits
WHERE board_id = ?
ORDER BY writer_uid, sequence
`, boardID.String())
	require.NoError(t, err)
	var markers []*boardhistory.Marker
	for rows.Next() {
		var encoded []byte
		require.NoError(t, rows.Scan(&encoded))
		marker, decodeErr := boardhistory.DecodeMarker(encoded)
		require.NoError(t, decodeErr)
		markers = append(markers, marker)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	mutations := make([][]boardhistory.Event, 0, len(markers))
	for _, marker := range markers {
		var events []boardhistory.Event
		for retained, err := range boardhistory.MutationEvents(
			marker,
			func(index int64) ([]byte, error) {
				var part []byte
				header := marker.Header()
				err := view.QueryRowContext(t.Context(), `
SELECT bytes
FROM board_mutation_parts
WHERE board_id = ? AND writer_uid = ? AND sequence = ? AND part_index = ?
`, boardID.String(), header.Writer.Bytes(), header.Sequence, index).Scan(&part)
				return part, err
			},
		) {
			require.NoError(t, err)
			events = append(events, retained.Event)
		}
		mutations = append(mutations, events)
	}
	return mutations
}

func projectHistoryKinds(events []boardhistory.Event) []boardhistory.EventKind {
	kinds := make([]boardhistory.EventKind, len(events))
	for index, event := range events {
		kinds[index] = event.Kind()
	}
	return kinds
}

func assertBoardConfigurationCleared(t *testing.T, events []boardhistory.Event) {
	t.Helper()
	require.Len(t, events, 5)
	prefix, ok := events[0].Payload.(*boardhistory.BoardIssueIDPrefixSet)
	require.True(t, ok)
	assert.Nil(t, prefix.Prefix)
	strategy, ok := events[1].Payload.(*boardhistory.BoardIssueIDStrategySet)
	require.True(t, ok)
	assert.Nil(t, strategy.Strategy)
	for _, limit := range []*int64{
		events[2].Payload.(*boardhistory.BoardSummaryLimitSet).Limit,
		events[3].Payload.(*boardhistory.BoardAttachmentLimitSet).Limit,
		events[4].Payload.(*boardhistory.BoardPinLimitSet).Limit,
	} {
		assert.Nil(t, limit)
	}
}

func installProjectHistoryFailure(t *testing.T, persistence *store.Store) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
CREATE TRIGGER fail_project_history_append
BEFORE INSERT ON board_mutation_commits
WHEN NEW.sequence > 1
BEGIN
    SELECT RAISE(ABORT, 'history failure');
END
`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}
