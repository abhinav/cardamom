package issue

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUnresolvedTitlePreservesOtherFields(t *testing.T) {
	uid, err := GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	state, err := Load(Snapshot{
		UID: uid, ID: "cm-one", Kind: new(KindTask),
		Lifecycle: new(LifecycleOpen), Priority: new(PriorityNormal),
		Created: now, Updated: now,
		Conflicts: []ConflictReference{{ID: "conflict_title", Field: FieldTitle}},
	})
	require.NoError(t, err)
	assert.Nil(t, state.Title())
	assert.Equal(t, new(KindTask), state.Kind())
	assert.Equal(t, new(PriorityNormal), state.Priority())
	assert.Equal(t, "conflict_title", state.Conflicts()[0].ID)
}

func TestLoadRejectsHiddenWinnerForUnresolvedTitle(t *testing.T) {
	uid, err := GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	_, err = Load(Snapshot{
		UID: uid, ID: "cm-one", Title: new("Hidden winner"), Kind: new(KindTask),
		Lifecycle: new(LifecycleOpen), Priority: new(PriorityNormal),
		Created: now, Updated: now,
		Conflicts: []ConflictReference{{ID: "conflict_title", Field: FieldTitle}},
	})
	assert.ErrorContains(t, err, "unresolved title cannot have a current value")
}
