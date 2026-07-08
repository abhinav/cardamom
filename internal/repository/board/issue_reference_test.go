package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepositoryResolveReferencesReturnsEmptyCollections(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "owner-",
		IDStrategy: "sequential",
	})

	issues, err := repository.ResolveIssueReferences(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, []issue.ID{}, issues)

	logs, err := repository.ResolveLogReferences(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, []issue.LogReference{}, logs)
}

func TestRepositoryResolveIssueReferencesScopesMembershipToBoard(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "owner-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Known issue", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)

	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-other', 'project-test', 'Other board', 1700000000)
	`)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issues (
			id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES (
			'other-1', 'board-other', 'Other issue', 'task', 'open', 2,
			1700000000, 1700000000
		)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	references, err := repository.ResolveIssueReferences(t.Context(), []issue.ID{
		"owner-1",
		"other-1",
		"unknown-1",
	})
	require.NoError(t, err)

	assert.Equal(t, []issue.ID{"owner-1"}, references)
}
