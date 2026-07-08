package issue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestValuesRejectInvalidRepresentations(t *testing.T) {
	t.Parallel()

	_, err := NewKind("bug")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = NewPriority(5)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = NewLabel(" ")
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestNewLabelRejectsReservedPrefixes(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"+phase:build", "-phase:build", " +phase:build", " -phase:build"} {
		_, err := NewLabel(value)
		assert.Equal(t, errkind.InvalidInput, errkind.Of(err), value)
	}
}

func TestNewIDEnforcesIssueIDGrammar(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"a", "A1", "cm-j26jb", "9-", "a--b"} {
		id, err := NewID(value)
		assert.NoError(t, err, value)
		assert.Equal(t, ID(value), id)
	}
	for _, value := range []string{
		"", "-issue", "issue_id", "issue/id", " issue", "issue ", "issue.name", "é",
	} {
		_, err := NewID(value)
		assert.Equal(t, errkind.InvalidInput, errkind.Of(err), value)
	}
}

func TestEnumValuesExposeBoundaryTextAndLifecycleMappings(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "workstream", KindWorkstream.String())
	assert.Empty(t, Kind(255).String())
	assert.Equal(t, "in_progress", StatusInProgress.String())
	assert.Equal(t, LifecycleOpen, StatusInProgress.Lifecycle())
	assert.Equal(t, LifecycleClosed, StatusClosed.Lifecycle())
	assert.Equal(t, "cancelled", LifecycleCancelled.String())
}

func TestValidValueCollectionsCannotMutatePolicy(t *testing.T) {
	t.Parallel()

	kinds := ValidKinds()
	kinds[0] = Kind(255)
	_, err := NewKind("workstream")
	assert.NoError(t, err)

	statuses := ValidStatuses()
	statuses[0] = Status(255)
	_, err = NewStatus("ready")
	assert.NoError(t, err)
}

func TestWaitingStateRequiresSingleLinePlainText(t *testing.T) {
	t.Parallel()

	waiting, err := NewWaitingState(" Root review ", time.Unix(10, 0).UTC())
	require.NoError(t, err)
	assert.Equal(t, "Root review", waiting.Reason)

	for _, reason := range []string{"", "   ", "first\nsecond", "tab\tseparated"} {
		_, err := NewWaitingState(reason, time.Unix(10, 0).UTC())
		assert.Error(t, err, reason)
	}
}

func TestStateSnapshotLinkDoesNotMutateOriginalState(t *testing.T) {
	t.Parallel()

	updatedAt := time.Unix(10, 0).UTC()
	state, err := Load(Snapshot{
		ID:        MustID("an-1"),
		Title:     "Snapshot linking",
		Kind:      KindTask,
		Lifecycle: LifecycleOpen,
		Priority:  PriorityNormal,
		Created:   updatedAt,
		Updated:   updatedAt,
		RecoveryState: &RecoveryState{
			Body: "Current recovery position.", Author: "author",
			UpdatedAt: &updatedAt,
		},
	})
	require.NoError(t, err)
	snapshotID, err := NewLogID("log_11111111111111111111111111111111")
	require.NoError(t, err)

	linked := state.WithRecoveryStateSnapshot(&snapshotID)

	require.NotNil(t, state.RecoveryStateRecord())
	assert.Nil(t, state.RecoveryStateRecord().SnapshotLogEntryID)
	require.NotNil(t, linked.RecoveryStateRecord())
	assert.Equal(
		t,
		&snapshotID,
		linked.RecoveryStateRecord().SnapshotLogEntryID,
	)
}

func TestWorkstreamIsCanonicalAndExecutable(t *testing.T) {
	t.Parallel()

	kind, err := NewKind("workstream")
	require.NoError(t, err)
	assert.Equal(t, KindWorkstream, kind)
	assert.True(t, kind.Executable())
	assert.Equal(t, []Kind{
		KindWorkstream, KindTask, KindCheckpoint, KindRoutine,
	}, ValidKinds())
	_, err = NewKind("decision")
	assert.ErrorContains(t, err, `invalid type "decision"`)
	_, err = NewKind("mission")
	assert.ErrorContains(t, err, `invalid type "mission"`)
	_, err = NewKind("epic")
	assert.ErrorContains(t, err, `invalid type "epic"`)
}
