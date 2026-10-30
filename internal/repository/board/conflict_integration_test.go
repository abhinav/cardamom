package board

import (
	"crypto/rand"
	"testing"
	"time"

	domainboard "go.abhg.dev/cardamom/internal/board"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/configuration/yamlstore"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
)

func TestConflictIntegrationReadResolveAndStaleReview(t *testing.T) {
	f := seedConflictFixture(t, t.TempDir())
	page, err := f.repository.ListConflicts(t.Context(), boardconflict.ListRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Conflicts, 2)
	require.NotEmpty(t, page.Next)
	next, err := f.repository.ListConflicts(t.Context(), boardconflict.ListRequest{Limit: 10, After: page.Next})
	require.NoError(t, err)
	assert.Len(t, next.Conflicts, 3)
	filtered, err := f.repository.ListConflicts(t.Context(), boardconflict.ListRequest{Limit: 10, IssueID: "demo-title"})
	require.NoError(t, err)
	assert.Len(t, filtered.Conflicts, 2)
	view, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-title"})
	require.NoError(t, err)
	assert.Nil(t, view.Detail.Issue.Title)
	assert.Equal(t, "ready", view.Detail.Issue.Status)
	assert.Equal(t, new("Known Details remain readable while other fields are disputed."), view.Detail.Issue.Details)
	assert.Len(t, view.Detail.Issue.Conflicts, 2)
	id := f.entries["title"].ID()
	inspected, err := f.repository.ReadConflict(t.Context(), id)
	require.NoError(t, err)
	require.Len(t, inspected.Issues, 1)
	assert.Nil(t, inspected.Issues[0].Reference.Title)
	assert.Equal(t, "ready", inspected.Issues[0].Reference.Status)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: id, Choice: boardconflict.TakeAlternative{ID: f.first["summary"]}})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	value, err := f.repository.ReadAlternative(t.Context(), id, f.first["title"])
	require.NoError(t, err)
	assert.Equal(t, "Title from first replica", value.Payload.(*boardhistory.IssueTitleSet).Title)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: id, Choice: boardconflict.TakeAlternative{ID: f.first["title"]}})
	require.NoError(t, err)
	view, err = f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-title"})
	require.NoError(t, err)
	assert.Equal(t, new("Title from first replica"), view.Detail.Issue.Title)
	assert.Len(t, view.Detail.Issue.Conflicts, 1)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: id, Choice: boardconflict.ReplaceText{Body: "Stale edit"}})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	view, err = f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-title"})
	require.NoError(t, err)
	assert.Equal(t, new("Title from first replica"), view.Detail.Issue.Title)
}

func TestConflictIntegrationRepairsRelationsAndExecution(t *testing.T) {
	f := seedConflictFixture(t, t.TempDir())
	_, err := f.repository.RepairRelations(t.Context(), boardconflict.RelationsRequest{ConflictID: f.entries["cycle"].ID()})
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
	_, err = f.repository.ReadConflict(t.Context(), f.entries["cycle"].ID())
	require.NoError(t, err)
	_, err = f.repository.RepairRelations(t.Context(), boardconflict.RelationsRequest{ConflictID: f.entries["cycle"].ID(), Remove: []boardconflict.AssignmentID{boardconflict.AssignmentID(f.first["cycle"])}})
	require.NoError(t, err)
	view, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	assert.Equal(t, "unresolved", view.Detail.Issue.Status)
	unresolved, err := f.repository.ListIssues(t.Context(), issue.ListRequest{Lifecycles: []string{"unresolved"}})
	require.NoError(t, err)
	require.Len(t, unresolved, 1)
	assert.Equal(t, "demo-execution", unresolved[0].Issue.ID)
	_, err = f.repository.RepairExecution(t.Context(), boardconflict.ExecutionRequest{ConflictID: f.entries["execution"].ID(), Waiting: boardconflict.ClearWaiting})
	require.NoError(t, err)
	view, err = f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	assert.Equal(t, "closed", view.Detail.Issue.Status)
	assert.Empty(t, view.Detail.Issue.Conflicts)
}

func TestConflictIntegrationCloneRetainsMetadataAndBodies(t *testing.T) {
	directory := t.TempDir()
	f := seedConflictFixture(t, directory)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{Source: f.store, Destination: destination, SourceBlobs: openCopyAttachmentRepository(t, f.store, directory), DestinationBlobs: destinationBlobs, Entropy: rand.Reader}
	_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-conflicts", ProjectID: "project-destination"})
	require.NoError(t, err)
	cloned, err := New(destination, Config{BoardID: "board-conflicts"})
	require.NoError(t, err)
	for name, entry := range f.entries {
		inspected, err := cloned.ReadConflict(t.Context(), entry.ID())
		require.NoError(t, err, name)
		assert.Equal(t, entry.ID(), inspected.Conflict.ID())
		original, err := f.repository.ReadAlternative(t.Context(), entry.ID(), f.first[name])
		require.NoError(t, err, name)
		copied, err := cloned.ReadAlternative(t.Context(), entry.ID(), f.first[name])
		require.NoError(t, err, name)
		assert.Equal(t, original.Payload, copied.Payload, name)
	}
	_, err = cloned.RepairExecution(t.Context(), boardconflict.ExecutionRequest{ConflictID: f.entries["execution"].ID(), Waiting: boardconflict.KeepWaiting, Lifecycle: new(issue.LifecycleOpen)})
	require.NoError(t, err)
	original, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	assert.Equal(t, "unresolved", original.Detail.Issue.Status)
	repaired, err := cloned.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	assert.Equal(t, "waiting", repaired.Detail.Issue.Status)
}

func TestConflictIntegrationConfigurationAndIndependentEdit(t *testing.T) {
	directory := t.TempDir()
	f := seedConflictFixture(t, directory)
	projectRepository := repositoryproject.New(f.store, repositoryproject.Config{})
	configs := configuration.NewService(&yamlstore.Store{Directory: directory}, projectRepository)
	effective, err := configs.ResolveConfiguration(t.Context(), f.repository.boardID)
	require.NoError(t, err)
	assert.Nil(t, effective.Issue.Summary.MaxBytes)
	assert.NotNil(t, effective.Attachment.MaxBytes)
	require.Len(t, effective.Conflicts, 1)
	assert.Equal(t, configuration.FieldIssueSummaryMaxBytes, effective.Conflicts[0].Field)
	planner := planning.NewPlanner(f.repository, f.repository, &planning.PlannerOptions{BoardID: f.repository.boardID, Configuration: configs})
	edited, err := planner.EditIssue(t.Context(), issue.NewInvocation("reviewer"), planning.EditIssueRequest{ID: "demo-title", Priority: new(1)})
	require.NoError(t, err)
	assert.Equal(t, new(1), edited.Issue.Issue.Priority)
	assert.Nil(t, edited.Issue.Issue.Title)
	assert.Len(t, edited.Issue.Issue.Conflicts, 2)
	_, err = planner.EditIssue(t.Context(), issue.NewInvocation("reviewer"), planning.EditIssueRequest{ID: "demo-independent", SummarySet: true, Summary: new("New Summary")})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: f.entries["limit"].ID(), Choice: boardconflict.TakeAlternative{ID: f.first["limit"]}})
	require.NoError(t, err)
	effective, err = configs.ResolveConfiguration(t.Context(), f.repository.boardID)
	require.NoError(t, err)
	require.NotNil(t, effective.Issue.Summary.MaxBytes)
	assert.Equal(t, uint64(4096), effective.Issue.Summary.MaxBytes.Uint64())
	assert.Empty(t, effective.Conflicts)
	_, err = planner.EditIssue(t.Context(), issue.NewInvocation("reviewer"), planning.EditIssueRequest{ID: "demo-independent", SummarySet: true, Summary: new("New Summary")})
	require.NoError(t, err)
}

func TestConflictIntegrationCheckpointChoiceKeepsDisputedLifecycle(t *testing.T) {
	f := seedConflictFixture(t, t.TempDir())
	uid := f.uids["demo-checkpoint"]
	change, err := f.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	queries := query.New(change)
	at := time.Unix(1700000100, 0).UTC()
	approved := f.appendConcurrentMutation(t, queries,
		&boardhistory.IssueCheckpoint{IssueUID: uid, Outcome: "approved", DecidedAt: at, Reason: "Approved by first replica"},
		&boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: "closed", ClosedAt: &at})
	denied := f.appendConcurrentMutation(t, queries,
		&boardhistory.IssueCheckpoint{IssueUID: uid, Outcome: "denied", DecidedAt: at, Reason: "Denied by second replica"},
		&boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: "cancelled", ClosedAt: &at})
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	// Model the first replica's published decision before the concurrent
	// second replica's decision is imported and both projections are hidden.
	for _, candidate := range approved {
		unit, err := boardconflict.UnitOf(candidate.Value)
		require.NoError(t, err)
		require.NoError(t, f.repository.writeConflictProjection(t.Context(), queries, unit, candidate.Value, nil, reservation.Revision()))
	}
	var decision *boardconflict.Conflict
	for index := range 2 {
		conflict, err := boardconflict.NewConflict(f.repository.boardID, func(yield func(boardconflict.Candidate, error) bool) {
			if yield(approved[index], nil) {
				yield(denied[index], nil)
			}
		})
		require.NoError(t, err)
		require.NoError(t, f.repository.retainConflict(t.Context(), queries, conflict, reservation.Revision()))
		if index == 0 {
			decision = conflict
		}
	}
	_, err = change.ExecContext(t.Context(), "UPDATE boards SET revision = ? WHERE id = ?", reservation.Revision(), f.repository.boardID.String())
	require.NoError(t, err)
	require.NoError(t, change.PublishRevision(t.Context(), reservation))
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	var selected string
	for _, alternative := range decision.Alternatives() {
		if alternative.Origins[0] == approved[0].Origins[0] {
			selected = alternative.ID
		}
	}
	require.NotEmpty(t, selected)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: decision.ID(), Choice: boardconflict.TakeAlternative{ID: selected}})
	require.NoError(t, err)
	inspected, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-checkpoint"})
	require.NoError(t, err)
	require.NotNil(t, inspected.Detail.CheckpointDecision)
	assert.Equal(t, "approved", inspected.Detail.CheckpointDecision.Outcome)
	assert.Nil(t, inspected.Detail.Issue.Lifecycle)
	require.Len(t, inspected.Detail.Issue.Conflicts, 1)
	assert.Equal(t, issue.FieldLifecycle, inspected.Detail.Issue.Conflicts[0].Field)
	view, err := f.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	for _, candidate := range []boardconflict.Candidate{approved[0], denied[0]} {
		event, err := repositoryhistory.ReadEvent(t.Context(), query.New(view), f.repository.boardID, candidate.Origins[0])
		require.NoError(t, err)
		assert.Equal(t, candidate.Value, event.Payload)
	}
}

func TestConflictIntegrationArchiveCanResolveThroughItsOwnGate(t *testing.T) {
	f := seedConflictFixture(t, t.TempDir())
	f.retain(t, "archive", false, &boardhistory.BoardArchiveSet{}, &boardhistory.BoardArchiveSet{Archive: &domainboard.Archive{Actor: "reviewer", At: time.Unix(1700000100, 0).UTC(), Reason: new("Completed on another replica")}})
	_, err := f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: f.entries["title"].ID(), Choice: boardconflict.TakeAlternative{ID: f.first["title"]}})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	_, err = f.repository.ReadConflict(t.Context(), f.entries["archive"].ID())
	require.NoError(t, err)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: f.entries["archive"].ID(), Choice: boardconflict.TakeAlternative{ID: f.first["archive"]}})
	require.NoError(t, err)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: f.entries["title"].ID(), Choice: boardconflict.TakeAlternative{ID: f.first["title"]}})
	require.NoError(t, err)
}

func TestConflictIntegrationResultChoiceKeepsExecutionConflict(t *testing.T) {
	f := seedConflictFixture(t, t.TempDir())
	uid := f.uids["demo-execution"]
	f.retain(t, "result", false, &boardhistory.IssueResult{IssueUID: uid, Body: new("Retained outcome")}, &boardhistory.IssueResult{IssueUID: uid})
	before, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	assert.Nil(t, before.Detail.CurrentResult)
	_, err = f.repository.ResolveConflict(t.Context(), boardconflict.ResolveRequest{ConflictID: f.entries["result"].ID(), Choice: boardconflict.TakeAlternative{ID: f.first["result"]}})
	require.NoError(t, err)
	after, err := f.repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "demo-execution"})
	require.NoError(t, err)
	require.NotNil(t, after.Detail.CurrentResult)
	assert.Equal(t, "Retained outcome", after.Detail.CurrentResult.Body)
	assert.Nil(t, after.Detail.Issue.Lifecycle)
	assert.Equal(t, "unresolved", after.Detail.Issue.Status)
	require.Len(t, after.Detail.Issue.Conflicts, 2)
	for _, reference := range after.Detail.Issue.Conflicts {
		assert.Equal(t, f.entries["execution"].ID(), reference.ID)
	}
}
