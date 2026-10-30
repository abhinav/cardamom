package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestStateUnresolvedNamePreservesIdentityAndIndependentEdits(t *testing.T) {
	state := testBoardState(t, Snapshot{
		ID: "board-one", ProjectID: "project-one", Created: time.Unix(10, 0),
		Conflicts: []ConflictReference{{ID: "conflict-name", Field: FieldName}},
	})
	assert.Nil(t, state.Name())
	selector, err := NewSelector("board-one")
	require.NoError(t, err)
	assert.True(t, selector.Matches(state))
	assert.NoError(t, state.RequireMutable())

	edited, err := state.EditSettings(SettingsEdit{Description: ReplaceDescription(new("Known description"))})
	require.NoError(t, err)
	assert.Equal(t, new("Known description"), edited.Description())
	assert.Equal(t, state.Conflicts(), edited.Conflicts())
	assert.Nil(t, edited.Name())
	_, err = state.EditSettings(SettingsEdit{Name: new("Pick a winner")})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.ErrorContains(t, err, "conflict-name")
}

func TestStateUnresolvedArchiveRejectsTransitions(t *testing.T) {
	state := testBoardState(t, Snapshot{
		ID: "board-one", ProjectID: "project-one", Name: new("Board"), Created: time.Unix(10, 0),
		Conflicts: []ConflictReference{{ID: "conflict-archive", Field: FieldArchive}},
	})
	assert.Nil(t, state.Archived())
	assert.True(t, state.Unresolved(FieldArchive))
	assert.Equal(t, errkind.Conflict, errkind.Of(state.RequireMutable()))
	_, err := state.ArchiveBoard("actor", time.Unix(20, 0), nil)
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = state.Unarchive()
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
}

func TestStateRequiresConflictForAbsentName(t *testing.T) {
	_, err := Load(Snapshot{ID: "board-one", ProjectID: "project-one", Created: time.Unix(10, 0)})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = Load(Snapshot{
		ID: "board-one", ProjectID: "project-one", Name: new("Provisional"), Created: time.Unix(10, 0),
		Conflicts: []ConflictReference{{ID: "conflict-name", Field: FieldName}},
	})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestStateUnresolvedDescriptionPreservesNameEdits(t *testing.T) {
	state := testBoardState(t, Snapshot{
		ID: "board-one", ProjectID: "project-one", Name: new("Board"), Created: time.Unix(10, 0),
		Conflicts: []ConflictReference{{ID: "conflict-description", Field: FieldDescription}},
	})
	edited, err := state.EditSettings(SettingsEdit{Name: new("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, new("Renamed"), edited.Name())
	assert.Nil(t, edited.Description())
	assert.True(t, edited.Unresolved(FieldDescription))
	_, err = state.EditSettings(SettingsEdit{Description: ReplaceDescription(nil)})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
}
