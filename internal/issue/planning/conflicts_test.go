package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestEditPolicyPreservesUnrelatedConflict(t *testing.T) {
	state := loadIssue(t, "an-1", issue.KindTask, issue.StatusReady)
	snapshot := state.Snapshot()
	snapshot.Title = nil
	snapshot.Conflicts = []issue.ConflictReference{{ID: "conflict-title", Field: issue.FieldTitle}}
	state, err := issue.Load(snapshot)
	require.NoError(t, err)
	policy, err := LoadEdit(EditSnapshot{
		BoardID: "board", Revision: 1, Issue: state, OccurredAt: time.Unix(20, 0),
	})
	require.NoError(t, err)

	out, err := policy.EditIssue(EditIssue{IssueID: state.ID(), Priority: new(issue.PriorityHigh)})
	require.NoError(t, err)
	assert.Equal(t, new(issue.PriorityHigh), out.Issue.Priority())
	assert.Nil(t, out.Issue.Title())
	assert.Equal(t, snapshot.Conflicts, out.Issue.Conflicts())
}

func TestEditPolicyRejectsOrdinaryConflictOverwrite(t *testing.T) {
	state := loadIssue(t, "an-1", issue.KindTask, issue.StatusReady)
	snapshot := state.Snapshot()
	snapshot.Title = nil
	snapshot.Conflicts = []issue.ConflictReference{{ID: "conflict-title", Field: issue.FieldTitle}}
	state, err := issue.Load(snapshot)
	require.NoError(t, err)
	policy, err := LoadEdit(EditSnapshot{
		BoardID: "board", Revision: 1, Issue: state, OccurredAt: time.Unix(20, 0),
	})
	require.NoError(t, err)

	_, err = policy.EditIssue(EditIssue{IssueID: state.ID(), Title: new("Chosen title")})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.ErrorContains(t, err, "conflict-title")
}

func TestEditPolicyDoesNotConsumeUnknownKindForDetails(t *testing.T) {
	state := loadIssue(t, "an-1", issue.KindTask, issue.StatusReady)
	snapshot := state.Snapshot()
	snapshot.Kind = nil
	snapshot.Conflicts = []issue.ConflictReference{{ID: "conflict-kind", Field: issue.FieldKind}}
	state, err := issue.Load(snapshot)
	require.NoError(t, err)
	policy, err := LoadEdit(EditSnapshot{
		BoardID: "board", Revision: 1, Issue: state, OccurredAt: time.Unix(20, 0),
	})
	require.NoError(t, err)

	out, err := policy.EditIssue(EditIssue{IssueID: state.ID(), DetailsSet: true, Details: "Still useful context"})
	require.NoError(t, err)
	assert.Equal(t, "Still useful context", out.Issue.Details())
	assert.Nil(t, out.Issue.Kind())
	assert.Equal(t, snapshot.Conflicts, out.Issue.Conflicts())
}

func TestApplyPreservesUnrelatedConflicts(t *testing.T) {
	state := loadIssue(t, "an-1", issue.KindTask, issue.StatusReady).Snapshot()
	state.Title = nil
	state.Kind = nil
	state.Conflicts = []issue.ConflictReference{
		{ID: "conflict-title", Field: issue.FieldTitle},
		{ID: "conflict-kind", Field: issue.FieldKind},
	}
	current, err := issue.Load(state)
	require.NoError(t, err)
	policy, err := LoadApply(ApplySnapshot{
		BoardID: "board", Revision: 1, Mode: ApplyModeCommit,
		IssueIDs:     []issue.ID{current.ID()},
		Issues:       map[issue.ID]ApplyIssueSnapshot{current.ID(): {State: current}},
		AllocatedIDs: []issue.ID{current.ID()}, AllocatedUIDs: []issue.UID{current.UID()},
		OccurredAt: time.Unix(20, 0),
	})
	require.NoError(t, err)
	command, err := applyDocumentCommand(ApplyDocumentRequest{
		Version: 1, Mode: ApplyModeCommit, OnExisting: ApplyExistingUpdate,
		Issues: []ApplyIssue{{ID: new(current.ID().String()), Details: new("Additional context")}},
	}, configuration.Defaults().Issue.Summary.MaxBytes)
	require.NoError(t, err)
	out, err := policy.ApplyDocument(command)
	require.NoError(t, err)
	require.Len(t, out.Applied, 1)
	assert.Nil(t, out.Applied[0].Issue.Title())
	assert.Nil(t, out.Applied[0].Issue.Kind())
	assert.Equal(t, "Additional context", out.Applied[0].Issue.Details())
	assert.Equal(t, state.Conflicts, out.Applied[0].Issue.Conflicts())
}
