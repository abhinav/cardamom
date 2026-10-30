package conflictconnect

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardhistory"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestConflictValuePreservesAbsenceAndRecordMetadata(t *testing.T) {
	inherited, err := encodeValue(&boardhistory.BoardPinLimitSet{})
	require.NoError(t, err)
	assert.Nil(t, inherited.GetOverrideLimit().Value)
	zero, err := encodeValue(&boardhistory.BoardPinLimitSet{Limit: new(int64(0))})
	require.NoError(t, err)
	assert.Equal(t, new(int64(0)), zero.GetOverrideLimit().Value)
	uid, err := issue.ParseUID([]byte("conflict-subject"))
	require.NoError(t, err)
	now := time.Date(2026, 9, 9, 1, 2, 3, 0, time.UTC)
	state, err := encodeValue(&boardhistory.IssueState{IssueUID: uid, State: &boardhistory.StateRecord{Body: "Position", Author: new("writer"), UpdatedAt: &now, NextAction: new("Continue"), SnapshotLogEntryID: new("log-1")}})
	require.NoError(t, err)
	assert.Equal(t, "Position", state.GetState().GetBody())
	assert.Equal(t, "Continue", state.GetState().GetNextAction())
	assert.Equal(t, "writer", state.GetState().GetAuthor())
	assert.Equal(t, now, state.GetState().GetUpdatedAt().AsTime())
	assert.Equal(t, "log-1", state.GetState().GetSnapshotLogEntryId())
	empty, err := encodeValue(&boardhistory.IssueState{IssueUID: uid})
	require.NoError(t, err)
	assert.Nil(t, empty.GetState().Body)
}

func TestRelationRequestPreservesCompoundParentChoices(t *testing.T) {
	value, err := relationRequest("reviewed", &v1.ConflictRelationsRepair{
		Remove: []string{"alt-dependency"}, Parents: []*v1.ConflictParentSelection{
			{ChildId: "cm-child", ParentId: new("cm-parent")}, {ChildId: "cm-other"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "reviewed", value.ConflictID)
	require.Len(t, value.Parents, 2)
	assert.Equal(t, issue.ID("cm-child"), value.Parents[0].ChildID)
	assert.Equal(t, new(issue.ID("cm-parent")), value.Parents[0].ParentID)
	assert.Nil(t, value.Parents[1].ParentID)
	assert.Equal(t, "alt-dependency", string(value.Remove[0]))
}

func TestExecutionRequestRequiresWaitingDecision(t *testing.T) {
	_, err := executionRequest("reviewed", &v1.ConflictExecutionRepair{})
	assert.ErrorContains(t, err, "waiting must be keep or clear")
	value, err := executionRequest("reviewed", &v1.ConflictExecutionRepair{Waiting: "clear"})
	require.NoError(t, err)
	assert.Nil(t, value.Kind)
	assert.Nil(t, value.Lifecycle)
	assert.Empty(t, value.WaitingAssignment)
}
