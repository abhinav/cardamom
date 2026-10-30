package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryBoardConflictsPreserveIndependentFields(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	boardID := namespace.Board.ID()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
UPDATE boards SET name = NULL, issue_id_prefix = NULL WHERE id = ?;
INSERT INTO board_conflicts (id, board_id, kind, revision) VALUES
    ('conflict-name', ?, 'board.name.set', 0),
    ('conflict-prefix', ?, 'board.issue_id_prefix.set', 0);
`, boardID.String(), boardID.String(), boardID.String())
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	repository := New(persistence, Config{})
	state, err := repository.Board(t.Context(), boardID)
	require.NoError(t, err)
	assert.Nil(t, state.Name())
	assert.ElementsMatch(t, []board.ConflictReference{
		{ID: "conflict-name", Field: board.FieldName},
		{ID: "conflict-prefix", Field: board.FieldIssueIDPrefix},
	}, state.Conflicts())
	all, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, state, all[0])
	active, err := repository.ListProjectBoards(t.Context(), namespace.Project.ID())
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, state, active[0])
	sole, err := repository.SoleBoard(t.Context())
	require.NoError(t, err)
	assert.Equal(t, state, sole)

	settings := configuration.NewService(emptyConfigurationStore{}, repository)
	resolved, err := settings.Resolve(t.Context(), boardID)
	require.NoError(t, err)
	assert.Nil(t, resolved.Effective.Issue.ID.Prefix)
	assert.Equal(t, configuration.ScopeBoard, resolved.Origins.Issue.ID.Prefix.Scope)
	assert.Equal(t, []configuration.ConflictReference{{ID: "conflict-prefix", Field: configuration.FieldIssueIDPrefix}}, resolved.Board.Conflicts)

	edited, err := repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID:  boardID,
		Settings: board.SettingsEdit{Description: board.ReplaceDescription(new("Independent description"))},
	})
	require.NoError(t, err)
	assert.Nil(t, edited.Name())
	assert.Equal(t, state.Conflicts(), edited.Conflicts())
	assert.Equal(t, new("Independent description"), edited.Description())
	limit, err := configuration.NewByteLimit(512)
	require.NoError(t, err)
	_, err = repository.UpdateBoardConfiguration(t.Context(), boardID, configuration.Patch{
		Fields:    []configuration.Field{configuration.FieldAttachmentMaxBytes},
		Overrides: configuration.Overrides{Attachment: configuration.AttachmentOverrides{MaxBytes: &limit}},
	})
	require.NoError(t, err)
	resolved, err = settings.Resolve(t.Context(), boardID)
	require.NoError(t, err)
	assert.Equal(t, &limit, resolved.Effective.Attachment.MaxBytes)
	assert.Nil(t, resolved.Effective.Issue.ID.Prefix)
	before := canonicalRevision(t, persistence)
	_, err = repository.UpdateBoardConfiguration(t.Context(), boardID, configuration.Patch{
		Fields: []configuration.Field{configuration.FieldIssueIDPrefix},
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID: boardID, Settings: board.SettingsEdit{Name: new("Replacement")},
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.Equal(t, before, canonicalRevision(t, persistence))
}

func TestRepositoryUnresolvedArchiveIsNeitherActiveNorArchived(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{Path: t.TempDir() + "/board.sqlite3"})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
	namespace := initializeProjectCatalog(t, persistence, new("Board"), Config{})
	boardID := namespace.Board.ID()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
INSERT INTO board_conflicts (id, board_id, kind, revision)
VALUES ('conflict-archive', ?, 'board.archive.set', 0);
`, boardID.String())
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	repository := New(persistence, Config{})
	state, err := repository.Board(t.Context(), boardID)
	require.NoError(t, err)
	assert.Nil(t, state.Archived())
	assert.True(t, state.Unresolved(board.FieldArchive))
	all, err := repository.ListAllBoards(t.Context())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, state, all[0])
	active, err := repository.ListProjectBoards(t.Context(), namespace.Project.ID())
	require.NoError(t, err)
	assert.Empty(t, active)
	_, err = repository.SoleBoard(t.Context())
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	before := canonicalRevision(t, persistence)
	_, err = repository.EditBoardSettings(t.Context(), board.EditRequest{
		BoardID: boardID, Settings: board.SettingsEdit{Description: board.ReplaceDescription(new("Description"))},
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = repository.UpdateBoardConfiguration(t.Context(), boardID, configuration.Patch{
		Fields: []configuration.Field{configuration.FieldAttachmentMaxBytes},
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = repository.ArchiveBoard(t.Context(), board.NewInvocation("actor"), board.ArchiveRequest{BoardID: boardID})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, _, err = repository.UnarchiveBoard(t.Context(), boardID)
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.Equal(t, before, canonicalRevision(t, persistence))
}
