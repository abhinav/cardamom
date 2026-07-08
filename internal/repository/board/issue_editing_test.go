package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepositoryEditsIssueMetadataAndGraphStateAtomically(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "edit-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)

	for _, title := range []string{"Program", "Prerequisite", "Child"} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
			Title: title, Type: "task", Priority: 2,
		})
		require.NoError(t, err)
	}

	workstream := "workstream"
	priority := 1
	summary := "Coordinate the repository rewrite."
	parent := "edit-1"
	edited, err := planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "edit-3", Title: new("Implementation"), Type: &workstream,
		Priority: &priority, Summary: &summary, SummarySet: true,
		Parent: &parent, ParentSet: true,
		AddDependencies: []string{"edit-2"}, AddLabels: []string{"backend", "urgent"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Implementation", edited.Issue.Issue.Title)
	assert.Equal(t, "workstream", edited.Issue.Issue.Type)
	assert.Equal(t, []string{"backend", "urgent"}, edited.Issue.Labels)
	require.Len(t, edited.Issue.DependsOn, 1)
	assert.Equal(t, "edit-2", edited.Issue.DependsOn[0].ID)
	assert.Equal(t, new("edit-1"), edited.Issue.ParentID)
	assert.Equal(t, int64(4), edited.Issue.Issue.Revision)

	unchanged, err := planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "edit-3", AddLabels: []string{"backend"},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(4), unchanged.Issue.Issue.Revision)
	removed, err := planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "edit-3", RemoveLabels: []string{"backend"},
		RemoveDependencies: []string{"edit-2"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"urgent"}, removed.Issue.Labels)
	assert.Empty(t, removed.Issue.DependsOn)
	assert.Equal(t, int64(5), removed.Issue.Issue.Revision)
	added, err := planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "edit-3", AddDependencies: []string{"edit-2"},
	})
	require.NoError(t, err)
	require.Len(t, added.Issue.DependsOn, 1)
	assert.Equal(t, "edit-2", added.Issue.DependsOn[0].ID)
	assert.Equal(t, int64(6), added.Issue.Issue.Revision)
}

func TestRepositoryEditRollsBackWhenRevisionPublicationFails(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "rollback-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Original", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		CREATE TRIGGER reject_edit_revision
		BEFORE UPDATE OF revision ON boards
		BEGIN
			SELECT RAISE(ABORT, 'reject edit revision');
		END
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	_, err = planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "rollback-1", Title: new("Leaked title"),
	})
	require.ErrorContains(t, err, "reject edit revision")

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "rollback-1"})
	require.NoError(t, err)
	assert.Equal(t, "Original", view.Detail.Issue.Title)
	assert.Equal(t, int64(1), view.Detail.Issue.Revision)

	change, err = repository.store.Change(t.Context())
	require.NoError(t, err)
	var revision int64
	require.NoError(t, change.QueryRowContext(t.Context(), `
		SELECT current_revision FROM store_state WHERE singleton = 1
	`).Scan(&revision))
	assert.Equal(t, int64(1), revision)
	assert.NoError(t, change.Done())
}
