package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
)

func TestRepositoryResolveLogReferencesScopesOwnershipToBoard(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "owner-",
		IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{
			Title: "Reference owner", Type: "task", Priority: 2,
		},
	)
	require.NoError(t, err)
	added, err := recorder.AddLogEntry(
		t.Context(),
		issue.NewInvocation("captain"),
		record.AddLogEntryRequest{
			IssueID: "owner-1", Body: "Owned by this board.",
		},
	)
	require.NoError(t, err)

	otherLogID, err := issue.NewLogID(
		"log_11111111111111111111111111111111",
	)
	require.NoError(t, err)
	unknownLogID, err := issue.NewLogID(
		"log_22222222222222222222222222222222",
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
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issue_log_entries (
			id, board_id, issue_id, kind, author, committer, body, created_at
		) VALUES (
			?, 'board-other', 'other-1', 'post', 'captain', 'captain',
			'Other board.', 1700000000
		)
	`, otherLogID)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	references, err := repository.ResolveLogReferences(t.Context(), []issue.LogID{
		added.LogEntry.ID,
		otherLogID,
		unknownLogID,
	})
	require.NoError(t, err)

	assert.Equal(t, []issue.LogReference{{
		LogID: added.LogEntry.ID, IssueID: "owner-1",
	}}, references)
}
