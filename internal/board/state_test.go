package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestStateEditSettings(t *testing.T) {
	current := "Shared context"
	state := testBoardState(t, Snapshot{
		ID:          testBoardID(t, "board-one"),
		ProjectID:   "project-one",
		Name:        "Development",
		Description: &current,
		Created:     time.Unix(10, 0).UTC(),
	})
	name := "  Release operations  "
	description := "# Readiness"

	edited, err := state.EditSettings(SettingsEdit{
		Name:        &name,
		Description: ReplaceDescription(&description),
	})
	require.NoError(t, err)

	assert.Equal(t, state.ID(), edited.ID())
	assert.Equal(t, state.ProjectID(), edited.ProjectID())
	assert.Equal(t, state.Created(), edited.Created())
	assert.Equal(t, "Release operations", edited.Name())
	assert.Equal(t, &description, edited.Description())
	assert.Equal(t, "Development", state.Name())
	assert.Equal(t, &current, state.Description())
}

func TestStateEditSettingsRejectsInvalidAtomicEdit(t *testing.T) {
	state := testBoardState(t, Snapshot{
		ID:        testBoardID(t, "board-one"),
		ProjectID: "project-one",
		Name:      "Development",
		Created:   time.Unix(10, 0).UTC(),
	})
	name := "Release operations"
	blank := "  "

	_, err := state.EditSettings(SettingsEdit{
		Name:        &name,
		Description: ReplaceDescription(&blank),
	})

	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	assert.Equal(t, "Development", state.Name())
}

func TestRevisionValidate(t *testing.T) {
	assert.NoError(t, Revision(0).Validate())
	assert.Equal(t, errkind.InvalidInput, errkind.Of(Revision(-1).Validate()))
}

func testBoardID(t *testing.T, value string) ID {
	t.Helper()
	id, err := NewID(value)
	require.NoError(t, err)
	return id
}

func testBoardState(t *testing.T, snapshot Snapshot) *State {
	t.Helper()
	state, err := Load(snapshot)
	require.NoError(t, err)
	return state
}
