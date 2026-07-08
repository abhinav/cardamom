package board

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
)

func TestRepositoryScalesRandomIssueIDsByPhysicalStorePopulation(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "random-",
		IDStrategy: "random", Entropy: bytes.NewReader(make([]byte, 64)),
	})
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-other', 'project-test', 'Other board', 1700000000)
	`)
	require.NoError(t, err)
	for number := range 512 {
		_, err = change.ExecContext(t.Context(), `
			INSERT INTO issues (
				id, board_id, title, kind, lifecycle, priority,
				created_at, updated_at
			) VALUES (?, 'board-other', 'Existing issue', 'task', 'open', 2, 1700000000, 1700000000)
		`, fmt.Sprintf("other-%d", number))
		require.NoError(t, err)
	}
	require.NoError(t, change.Commit())

	planner := planning.NewPlanner(repository, repository, nil)
	created, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Scaled random identity", Type: "task", Priority: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "random-aaaaa", created.Issue.Issue.ID)
}

func TestRepositoryApplyDocumentRetriesTransactionLocalRandomIDCollisions(t *testing.T) {
	var entropy []byte
	// Four-byte suffix chunks force a, a, then b.
	for _, value := range []byte{0, 0, 1} {
		entropy = append(entropy, bytes.Repeat([]byte{value}, 4)...)
	}
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "random-",
		IDStrategy: "random", Entropy: bytes.NewReader(entropy),
	})
	planner := planning.NewPlanner(repository, repository, nil)

	applied, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), planning.ApplyDocumentRequest{
		Version: 1, Mode: planning.ApplyModeCommit,
		Issues: []planning.ApplyIssue{
			{Alias: new("first"), Title: new("First issue"), Type: new("task")},
			{Alias: new("second"), Title: new("Second issue"), Type: new("task")},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, new("random-aaaa"), applied.Entries[0].ID)
	assert.Equal(t, new("random-bbbb"), applied.Entries[1].ID)

	ids, err := repository.ListIssueIDs(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"random-aaaa", "random-bbbb"}, ids)
}

func TestRepositoryFailedDocumentDoesNotReserveRandomIDs(t *testing.T) {
	var entropy []byte
	// Validation fails before allocation, so the next mutation receives a.
	for _, value := range []byte{0, 1, 0} {
		entropy = append(entropy, bytes.Repeat([]byte{value}, 4)...)
	}
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "random-",
		IDStrategy: "random", Entropy: bytes.NewReader(entropy),
	})
	planner := planning.NewPlanner(repository, repository, nil)

	dependenciesOnSecond := []planning.ApplyIssueReference{{
		Kind: planning.ApplyReferenceAlias, Alias: "second",
	}}
	dependenciesOnFirst := []planning.ApplyIssueReference{{
		Kind: planning.ApplyReferenceAlias, Alias: "first",
	}}
	_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), planning.ApplyDocumentRequest{
		Version: 1, Mode: planning.ApplyModeCommit,
		Issues: []planning.ApplyIssue{
			{
				Alias: new("first"), Title: new("First issue"), Type: new("task"),
				DependsOn: &dependenciesOnSecond,
			},
			{
				Alias: new("second"), Title: new("Second issue"), Type: new("task"),
				DependsOn: &dependenciesOnFirst,
			},
		},
	})
	require.ErrorContains(t, err, "dependency graph must remain acyclic")

	created, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "After failed graph", Type: "task", Priority: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "random-aaaa", created.Issue.Issue.ID)
}
