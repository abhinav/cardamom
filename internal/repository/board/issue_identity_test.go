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
	entropy := append(bytes.Repeat([]byte{0}, 5), bytes.Repeat([]byte{1}, 16)...)
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "random-",
		IDStrategy: "random", Entropy: bytes.NewReader(entropy),
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
				uid, id, board_id, title, kind, lifecycle, priority,
				created_at, updated_at
			) VALUES (randomblob(16), ?, 'board-other', 'Existing issue', 'task', 'open', 2, 1700000000, 1700000000)
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
	// Public suffix chunks force a, a, then b. Each created issue also consumes
	// a distinct private identity after its public suffix is accepted.
	entropy = append(entropy, bytes.Repeat([]byte{0}, 4)...)
	entropy = append(entropy, bytes.Repeat([]byte{1}, 16)...)
	entropy = append(entropy, bytes.Repeat([]byte{0}, 4)...)
	entropy = append(entropy, bytes.Repeat([]byte{1}, 4)...)
	entropy = append(entropy, bytes.Repeat([]byte{2}, 16)...)
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
	entropy = append(entropy, bytes.Repeat([]byte{0}, 4)...)
	entropy = append(entropy, bytes.Repeat([]byte{1}, 16)...)
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

func TestRepositoryCreateAndApplyAllocatePrivateUIDs(t *testing.T) {
	// The dry run must consume no bytes. The repeated second value makes the
	// second committed document issue retry a transaction-local collision.
	entropy := bytes.Join([][]byte{
		bytes.Repeat([]byte{1}, 16),
		bytes.Repeat([]byte{2}, 16),
		bytes.Repeat([]byte{2}, 16),
		bytes.Repeat([]byte{3}, 16),
	}, nil)
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "private-",
		IDStrategy: "sequential", Entropy: bytes.NewReader(entropy),
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(
		t.Context(),
		issue.NewInvocation("captain"),
		planning.CreateIssueRequest{Title: "Created issue", Type: "task", Priority: 2},
	)
	require.NoError(t, err)
	request := planning.ApplyDocumentRequest{
		Version: 1,
		Mode:    planning.ApplyModeDryRun,
		Issues: []planning.ApplyIssue{
			{Alias: new("first"), Title: new("First issue"), Type: new("task")},
			{Alias: new("second"), Title: new("Second issue"), Type: new("task")},
		},
	}

	_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), request)
	require.NoError(t, err)
	request.Mode = planning.ApplyModeCommit
	_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), request)
	require.NoError(t, err)

	view, err := repository.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var issueCount, uidCount int
	require.NoError(t, view.QueryRowContext(t.Context(), `
		SELECT count(*), count(DISTINCT hex(uid))
		FROM issues
		WHERE board_id = 'board-test'
	`).Scan(&issueCount, &uidCount))
	assert.Equal(t, 3, issueCount)
	assert.Equal(t, issueCount, uidCount)
}

func TestRepositoryPrivateUIDAllocationFailureRollsBack(t *testing.T) {
	t.Run("CreateEntropy", func(t *testing.T) {
		repository := openBoardRepository(t, Config{
			BoardID: mustBoardID(t, "board-test"), IDPrefix: "private-",
			IDStrategy: "sequential", Entropy: bytes.NewReader(nil),
		})
		planner := planning.NewPlanner(repository, repository, nil)

		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.CreateIssueRequest{Title: "No entropy", Type: "task", Priority: 2},
		)
		assert.ErrorContains(t, err, "read issue UID entropy")
		assert.Empty(t, mustListIssueIDs(t, repository))
		assertUnpublishedBoardRevision(t, repository)
	})

	t.Run("ApplyEntropy", func(t *testing.T) {
		repository := openBoardRepository(t, Config{
			BoardID: mustBoardID(t, "board-test"), IDPrefix: "private-",
			IDStrategy: "sequential",
			Entropy:    bytes.NewReader(bytes.Repeat([]byte{1}, 16)),
		})
		planner := planning.NewPlanner(repository, repository, nil)

		_, err := planner.ApplyDocument(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.ApplyDocumentRequest{
				Version: 1,
				Mode:    planning.ApplyModeCommit,
				Issues: []planning.ApplyIssue{
					{Alias: new("first"), Title: new("First"), Type: new("task")},
					{Alias: new("second"), Title: new("Second"), Type: new("task")},
				},
			},
		)
		assert.ErrorContains(t, err, "read issue UID entropy")
		assert.Empty(t, mustListIssueIDs(t, repository))
		assertUnpublishedBoardRevision(t, repository)
	})

	t.Run("ApplyCollisionLimit", func(t *testing.T) {
		collision := bytes.Repeat([]byte{6}, 16)
		repository := openBoardRepository(t, Config{
			BoardID: mustBoardID(t, "board-test"), IDPrefix: "private-",
			IDStrategy: "sequential",
			Entropy:    bytes.NewReader(bytes.Repeat(collision, 32)),
		})
		insertExistingIssueUID(t, repository, collision)
		planner := planning.NewPlanner(repository, repository, nil)

		_, err := planner.ApplyDocument(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.ApplyDocumentRequest{
				Version: 1,
				Mode:    planning.ApplyModeCommit,
				Issues: []planning.ApplyIssue{
					{Alias: new("first"), Title: new("First"), Type: new("task")},
					{Alias: new("second"), Title: new("Second"), Type: new("task")},
				},
			},
		)
		assert.ErrorContains(t, err, "private issue identity: collision limit reached")
		assert.Equal(t, []string{"existing"}, mustListIssueIDs(t, repository))
		assertUnpublishedBoardRevision(t, repository)
	})

	t.Run("CollisionLimit", func(t *testing.T) {
		collision := bytes.Repeat([]byte{7}, 16)
		repository := openBoardRepository(t, Config{
			BoardID: mustBoardID(t, "board-test"), IDPrefix: "private-",
			IDStrategy: "sequential",
			Entropy:    bytes.NewReader(bytes.Repeat(collision, 32)),
		})
		insertExistingIssueUID(t, repository, collision)
		planner := planning.NewPlanner(repository, repository, nil)

		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.CreateIssueRequest{Title: "Collision", Type: "task", Priority: 2},
		)
		assert.ErrorContains(t, err, "private issue identity: collision limit reached")
		assert.Equal(t, []string{"existing"}, mustListIssueIDs(t, repository))
		assertUnpublishedBoardRevision(t, repository)
	})
}

func mustListIssueIDs(t *testing.T, repository *Repository) []string {
	t.Helper()
	ids, err := repository.ListIssueIDs(t.Context())
	require.NoError(t, err)
	return ids
}

func insertExistingIssueUID(t *testing.T, repository *Repository, uid []byte) {
	t.Helper()
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO issues (
			uid, id, board_id, title, kind, lifecycle, priority,
			created_at, updated_at
		) VALUES (?, 'existing', 'board-test', 'Existing', 'task', 'open', 2, 1, 1)
	`, uid)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
}

func assertUnpublishedBoardRevision(t *testing.T, repository *Repository) {
	t.Helper()
	view, err := repository.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var storeRevision, boardRevision int64
	require.NoError(t, view.QueryRowContext(t.Context(), `
		SELECT store.current_revision, board.revision
		FROM store_state AS store
		JOIN boards AS board ON board.id = 'board-test'
		WHERE store.singleton = 1
	`).Scan(&storeRevision, &boardRevision))
	assert.Zero(t, storeRevision)
	assert.Zero(t, boardRevision)
}
