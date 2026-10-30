package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepositoryClaimNextReportsInheritedConflicts(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "pick-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Shared contract", Type: "workstream", Priority: 0},
		{Title: "Uncertain context", Type: "task", Priority: 0, Parent: "pick-1", Labels: []string{"selected"}},
		{Title: "Usable work", Type: "task", Priority: 1, Labels: []string{"selected"}},
		{Title: "Known blocker", Type: "task", Priority: 0, Parent: "pick-1", DependsOn: []string{"pick-3"}, Labels: []string{"selected"}},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), request)
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "parent-summary", boardhistory.EventIssueSummarySet, "pick-1", issue.FieldSummary)

	executor := execution.NewExecutor(r, r)
	request := execution.ClaimNextRequest{Assignee: "worker", LabelsAll: []string{"selected"}}
	claimed, err := executor.ClaimNext(t.Context(), issue.NewInvocation("worker"), request)
	require.NoError(t, err)
	require.NotNil(t, claimed.Issue)
	assert.Equal(t, "pick-3", claimed.Issue.Detail.Issue.ID)
	assert.Equal(t, 1, claimed.SkippedConflicts)

	before, err := r.ListIssuesSnapshot(t.Context(), issue.ListRequest{})
	require.NoError(t, err)
	empty, err := executor.ClaimNext(t.Context(), issue.NewInvocation("worker"), request)
	require.NoError(t, err)
	assert.Nil(t, empty.Issue)
	assert.Equal(t, 1, empty.SkippedConflicts)
	after, err := r.ListIssuesSnapshot(t.Context(), issue.ListRequest{})
	require.NoError(t, err)
	assert.Equal(t, before.Cursor, after.Cursor, "a conflict-only pool does not publish a revision")
}

func TestRepositoryClaimNextRetainsUncertainContainmentScope(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "scope-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Selected scope", Type: "workstream", Priority: 0},
		{Title: "Disputed ancestor", Type: "workstream", Priority: 0},
		{Title: "Possible descendant", Type: "task", Priority: 0, Parent: "scope-2", Labels: []string{"selected"}},
		{Title: "Known outside scope", Type: "task", Priority: 0, Labels: []string{"selected"}},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), request)
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "ancestor-parent", boardhistory.EventIssueContainment, "scope-2", issue.FieldParent)
	result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{
		Assignee: "worker", UnderID: "scope-1", LabelsAll: []string{"selected"},
	})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 1, result.SkippedConflicts)
}

func TestRepositoryPresentationDistinguishesUnknownAndBlockingPrerequisites(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "dependency-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Unknown prerequisite", Type: "task", Priority: 0},
		{Title: "Known open prerequisite", Type: "task", Priority: 0},
		{Title: "Uncertain readiness", Type: "task", Priority: 0, DependsOn: []string{"dependency-1"}, Labels: []string{"selected"}},
		{Title: "Known blocked", Type: "task", Priority: 0, DependsOn: []string{"dependency-1", "dependency-2"}, Labels: []string{"selected"}},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), request)
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "prerequisite-lifecycle", boardhistory.EventIssueLifecycleSet, "dependency-1", issue.FieldLifecycle)
	change, err := r.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `UPDATE issues SET lifecycle = NULL WHERE id = 'dependency-1'`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	view, err := r.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "dependency-3"})
	require.NoError(t, err)
	assert.Equal(t, "unresolved", view.Detail.Issue.Status)
	require.Len(t, view.Detail.DependsOn, 1)
	assert.Equal(t, "unresolved", view.Detail.DependsOn[0].Status)
	assert.Equal(t, []issue.ConflictReference{{ID: "prerequisite-lifecycle", Field: issue.FieldLifecycle}}, view.Detail.EligibilityConflicts)
	blocked, err := r.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "dependency-4"})
	require.NoError(t, err)
	assert.True(t, blocked.Detail.Blocked)
	assert.Equal(t, "blocked", blocked.Detail.Issue.Status)

	result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{
		Assignee: "worker", LabelsAll: []string{"selected"},
	})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 1, result.SkippedConflicts)
}

func TestRepositoryClaimNextCountsOnlyOtherwisePossibleWork(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "eligible-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Unknown kind", Type: "task", Priority: 0, Labels: []string{"selected"}},
		{Title: "Unknown lifecycle", Type: "task", Priority: 0, Labels: []string{"selected"}},
		{Title: "Known waiting despite unknown lifecycle", Type: "task", Priority: 0, Labels: []string{"selected"}},
		{Title: "Routine with disputed title", Type: "routine", Priority: 0, Labels: []string{"selected"}},
		{Title: "Checkpoint with disputed title", Type: "checkpoint", Priority: 0, Labels: []string{"selected"}},
		{Title: "Known missing requested label", Type: "task", Priority: 0},
		{Title: "Known closed", Type: "task", Priority: 0, Labels: []string{"selected"}},
		{Title: "Known claimed", Type: "task", Priority: 0, Labels: []string{"selected"}},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), request)
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "unknown-kind", boardhistory.EventIssueKindSet, "eligible-1", issue.FieldKind)
	for _, id := range []string{"eligible-2", "eligible-3"} {
		installPresentationConflict(t, r, id+"-lifecycle", boardhistory.EventIssueLifecycleSet, id, issue.FieldLifecycle)
	}
	for _, id := range []string{"eligible-4", "eligible-5", "eligible-6", "eligible-7", "eligible-8"} {
		installPresentationConflict(t, r, id+"-title", boardhistory.EventIssueTitleSet, id, issue.FieldTitle)
	}
	change, err := r.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		UPDATE issues SET kind = NULL WHERE id = 'eligible-1';
		UPDATE issues SET lifecycle = NULL WHERE id IN ('eligible-2', 'eligible-3');
		UPDATE issues SET waiting_reason = 'Waiting on access', waiting_since = 1700000000 WHERE id = 'eligible-3';
		UPDATE issues SET title = NULL WHERE id IN ('eligible-4', 'eligible-5', 'eligible-6', 'eligible-7', 'eligible-8');
		UPDATE issues SET lifecycle = 'closed', closed_at = 1700000000 WHERE id = 'eligible-7';
		INSERT INTO active_claims (issue_uid, board_id, actor, started_at, started_revision)
		SELECT uid, board_id, 'owner', 1700000000, 1 FROM issues WHERE id = 'eligible-8';
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{
		Assignee: "worker", LabelsAll: []string{"selected"},
	})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 2, result.SkippedConflicts)
}

func TestRepositoryClaimNextDistinguishesDisputedAndMissingLabels(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "label-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, title := range []string{"Requested label is disputed", "Unrelated label is disputed"} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), planning.CreateIssueRequest{Title: title, Type: "task", Priority: 0})
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "requested-label", boardhistory.EventIssueLabel, "label-1", issue.FieldLabels)
	installPresentationConflict(t, r, "other-label", boardhistory.EventIssueLabel, "label-2", issue.FieldLabels)
	change, err := r.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		UPDATE board_conflicts SET member_key = 'selected' WHERE id = 'requested-label';
		UPDATE board_conflicts SET member_key = 'unrelated' WHERE id = 'other-label';
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{
		Assignee: "worker", LabelsAll: []string{"selected"},
	})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 1, result.SkippedConflicts)
}

func TestRepositoryClaimNextPreservesPinnedContextUncertainty(t *testing.T) {
	r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "pin-", IDStrategy: "sequential"})
	planner := planning.NewPlanner(r, r, nil)
	for _, request := range []planning.CreateIssueRequest{
		{Title: "Pinned context", Type: "workstream", Priority: 0},
		{Title: "Selected work", Type: "task", Priority: 0, Labels: []string{"selected"}},
	} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("planner"), request)
		require.NoError(t, err)
	}
	installPresentationConflict(t, r, "pinned-summary", boardhistory.EventIssueSummarySet, "pin-1", issue.FieldSummary)
	change, err := r.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `INSERT INTO board_pins (board_id, issue_uid, position) SELECT board_id, uid, 1 FROM issues WHERE id = 'pin-1'`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{
		Assignee: "worker", LabelsAll: []string{"selected"},
	})
	require.NoError(t, err)
	assert.Nil(t, result.Issue)
	assert.Equal(t, 1, result.SkippedConflicts)
}

func TestRepositoryClaimNextReportsSharedBoardContextConflicts(t *testing.T) {
	for _, test := range []struct {
		name  string
		kind  boardhistory.EventKind
		field issue.Field
	}{
		{name: "Description", kind: boardhistory.EventBoardDescriptionSet, field: issue.FieldBoardDescription},
		{name: "Pins", kind: boardhistory.EventBoardPin, field: issue.FieldPins},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := openBoardRepository(t, Config{BoardID: mustBoardID(t, "board-test"), IDPrefix: "context-", IDStrategy: "sequential"})
			_, err := planning.NewPlanner(r, r, nil).CreateIssue(t.Context(), issue.NewInvocation("planner"), planning.CreateIssueRequest{
				Title: "Ready task", Type: "task", Priority: 0,
			})
			require.NoError(t, err)
			change, err := r.store.Change(t.Context())
			require.NoError(t, err)
			_, err = change.ExecContext(t.Context(), `
				INSERT INTO board_conflicts (id, board_id, kind, issue_uid, revision)
				SELECT 'shared-context', board_id, ?, CASE WHEN ? = 'board.pin' THEN uid END, 1
				FROM issues WHERE id = 'context-1'
			`, string(test.kind), string(test.kind))
			require.NoError(t, err)
			require.NoError(t, change.Commit())
			require.NoError(t, change.Done())
			view, err := r.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "context-1"})
			require.NoError(t, err)
			assert.Equal(t, []issue.ConflictReference{{ID: "shared-context", Field: test.field}}, view.Detail.EligibilityConflicts)
			result, err := execution.NewExecutor(r, r).ClaimNext(t.Context(), issue.NewInvocation("worker"), execution.ClaimNextRequest{Assignee: "worker"})
			require.NoError(t, err)
			assert.Nil(t, result.Issue)
			assert.Equal(t, 1, result.SkippedConflicts)
		})
	}
}

// installPresentationConflict stores only compact read metadata. These tests
// exercise selection and presentation; conflict resolution owns contender bodies.
func installPresentationConflict(t *testing.T, r *Repository, id string, kind boardhistory.EventKind, issueID string, field issue.Field) {
	t.Helper()
	change, err := r.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO board_conflicts (id, board_id, kind, issue_uid, member_key, revision)
		SELECT ?, board_id, ?, uid, '', 1 FROM issues WHERE id = ?
	`, id, string(kind), issueID)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO board_conflict_issues (conflict_id, issue_uid, field)
		SELECT ?, uid, ? FROM issues WHERE id = ?
	`, id, field.String(), issueID)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
}
