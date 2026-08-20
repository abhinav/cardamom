package board

import (
	"context"
	"database/sql"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestNewRejectsPrefixThatWouldGenerateInvalidIssueID(t *testing.T) {
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(t.TempDir(), "board.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	_, err = New(persistence, Config{
		BoardID:  mustBoardID(t, "board-test"),
		IDPrefix: "-",
	})
	assert.ErrorContains(t, err, "start with a letter or digit")
}

func TestRepositoryCreatesAndReadsBoardIssues(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)

	first, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title:    "Program",
		Type:     "workstream",
		Priority: 1,
		Labels:   []string{"program"},
	})
	require.NoError(t, err)
	assert.Equal(t, "task-1", first.Issue.Issue.ID)

	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title:     "Implementation",
		Type:      "task",
		Priority:  2,
		Labels:    []string{"backend", "urgent"},
		DependsOn: []string{"task-1"},
		Parent:    "task-1",
		Summary:   "Replace board persistence.",
	})
	require.NoError(t, err)
	recorder := record.NewRecorder(repository, repository)
	_, err = recorder.SetState(
		t.Context(),
		issue.NewInvocation("captain"),
		record.SetStateRequest{
			IssueID: "task-2",
			Text:    "Begin with repository operations.",
		},
	)
	require.NoError(t, err)
	view, err := repository.ReadIssue(
		t.Context(),
		issue.ReadRequest{IssueID: "task-2"},
	)
	require.NoError(t, err)

	assert.Equal(t, issue.Detail{
		Issue: issue.Issue{
			ID: "task-2", Title: "Implementation", Type: "task",
			Lifecycle: "open", Status: "blocked", Priority: 2,
			ActiveClaim: nil,
			Created:     1_700_000_000,
			Updated:     1_700_000_000,
			Summary:     new("Replace board persistence."),
			State:       new("Begin with repository operations."),
			Revision:    3,
		},
		State: &issue.RecoveryState{
			Body:      "Begin with repository operations.",
			Author:    "captain",
			UpdatedAt: new(time.Unix(1_700_000_000, 0).UTC()),
		},
		Labels:     []string{"backend", "urgent"},
		DependsOn:  []issue.Reference{{ID: "task-1", Title: "Program", Type: "workstream", Status: "ready", Priority: 1}},
		Blocks:     []issue.Reference{},
		LogSummary: issue.LogSummary{},
		ParentID:   new("task-1"),
		Story: issue.Story{
			Containment: []issue.ContainmentNode{
				{ID: "task-1", Title: "Program", Type: "workstream", Status: "ready", Priority: 1},
				{ID: "task-2", Title: "Implementation", Type: "task", Status: "blocked", Priority: 2, ParentID: new("task-1")},
			},
			DependsOn: []issue.Reference{{ID: "task-1", Title: "Program", Type: "workstream", Status: "ready", Priority: 1}},
			Blocks:    []issue.Reference{},
		},
		Blocked: true,
	}, view.Detail)

	listed, err := repository.ListIssues(t.Context(), issue.ListRequest{})
	require.NoError(t, err)
	require.Len(t, listed, 2)
	assert.Equal(t, []string{"task-1", "task-2"}, []string{listed[0].Issue.ID, listed[1].Issue.ID})
	assert.False(t, listed[0].Blocked)
	assert.True(t, listed[1].Blocked)

	depth := 0
	view, err = repository.ReadIssue(t.Context(), issue.ReadRequest{
		IssueID: "task-2", ContextDepth: &depth,
	})
	require.NoError(t, err)
	require.NotNil(t, view.Context)
	require.Len(t, view.Context.Ancestors, 1)
	assert.Equal(t, "task-1", view.Context.Ancestors[0].Issue.ID)
}

func TestRepositoryBindsAndReadsDirectIssueKeys(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)

	first, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "First", Type: "task", Priority: 2, Key: new("source:z"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"source:z"}, first.Issue.Keys)

	key := "source:a"
	edited, err := planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "task-1", Key: &key,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"source:a", "source:z"}, edited.Issue.Keys)
	boundRevision := edited.Issue.Issue.Revision

	edited, err = planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "task-1", Key: &key,
	})
	require.NoError(t, err)
	assert.Equal(t, boundRevision, edited.Issue.Issue.Revision)

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{Key: "source:z"})
	require.NoError(t, err)
	assert.Equal(t, "task-1", view.Detail.Issue.ID)
	assert.Equal(t, []string{"source:a", "source:z"}, view.Detail.Keys)
	resolved, err := repository.ResolveExternalKey(t.Context(), "source:z")
	require.NoError(t, err)
	assert.Equal(t, issue.ID("task-1"), resolved)
	_, err = repository.ResolveExternalKey(t.Context(), "")
	assert.ErrorContains(t, err, "external key required")

	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Second", Type: "task", Priority: 2, Key: new("source:other"),
	})
	require.NoError(t, err)
	otherKey := "source:a"
	_, err = planner.EditIssue(t.Context(), issue.NewInvocation("captain"), planning.EditIssueRequest{
		ID: "task-2", Key: &otherKey,
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))

	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Rejected", Type: "task", Priority: 2, Key: new("source:a"),
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	third, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Third", Type: "task", Priority: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "task-3", third.Issue.Issue.ID)
}

func TestRepositoryReadsOnlyRequestedExternalKeyOwner(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "First", Type: "task", Priority: 2, Key: new("source:first"),
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Second", Type: "task", Priority: 2, Key: new("source:second"),
	})
	require.NoError(t, err)

	view, err := repository.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	scope := &countingQueryScope{queryScope: view}

	owner, err := repository.readExternalKeyOwner(t.Context(), scope, nil)
	require.NoError(t, err)
	assert.Nil(t, owner)
	assert.Zero(t, scope.calls)

	key := planning.ExternalKey("source:first")
	owner, err = repository.readExternalKeyOwner(t.Context(), scope, &key)
	require.NoError(t, err)
	assert.Equal(t, &planning.ExternalKeyOwner{
		Key: key, IssueID: issue.MustID("task-1"),
	}, owner)
	assert.Equal(t, 1, scope.calls)

	missing := planning.ExternalKey("source:missing")
	owner, err = repository.readExternalKeyOwner(t.Context(), scope, &missing)
	require.NoError(t, err)
	assert.Nil(t, owner)
	assert.Equal(t, 2, scope.calls)
}

func TestRepositoryListIssueQueryCountDoesNotGrowWithBoardSize(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)

	create := func(title string) {
		t.Helper()
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
			Title: title, Type: "task", Priority: 2,
		})
		require.NoError(t, err)
	}
	countListQueries := func() int {
		t.Helper()
		view, err := repository.store.View(t.Context())
		require.NoError(t, err)
		scope := &countingQueryScope{queryScope: view}
		_, err = repository.listIssues(t.Context(), scope, issue.ListRequest{})
		require.NoError(t, err)
		require.NoError(t, view.Done())
		return scope.calls
	}

	create("First")
	oneIssueCalls := countListQueries()
	for _, title := range []string{"Second", "Third", "Fourth", "Fifth"} {
		create(title)
	}
	assert.Equal(t, oneIssueCalls, countListQueries())
}

func TestRepositoryListIssuesMatchesRepeatedProtocolFilters(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	executor := execution.NewExecutor(repository, repository)

	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Completed workstream", Type: "workstream", Priority: 1,
	})
	require.NoError(t, err)
	_, err = executor.CloseIssues(t.Context(), issue.NewInvocation("captain"), execution.CloseIssuesRequest{
		IDs: []string{"task-1"},
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Open task", Type: "task", Priority: 2,
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Open prerequisite", Type: "task", Priority: 2,
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Blocked task", Type: "task", Priority: 2, DependsOn: []string{"task-3"},
	})
	require.NoError(t, err)

	openTasks, err := repository.ListIssues(t.Context(), issue.ListRequest{
		Lifecycles: []string{"open"}, Types: []string{"task"},
	})
	require.NoError(t, err)
	require.Len(t, openTasks, 3)
	assert.Equal(t, []string{"task-2", "task-3", "task-4"}, summaryIDs(openTasks))

	blockedTasks, err := repository.ListIssues(t.Context(), issue.ListRequest{
		Lifecycles: []string{"open"}, Statuses: []string{"blocked"}, Types: []string{"task"},
	})
	require.NoError(t, err)
	require.Len(t, blockedTasks, 1)
	assert.Equal(t, "task-4", blockedTasks[0].Issue.ID)

	closedWorkstreams, err := repository.ListIssues(t.Context(), issue.ListRequest{
		Lifecycles: []string{"closed"}, Types: []string{"workstream"},
	})
	require.NoError(t, err)
	require.Len(t, closedWorkstreams, 1)
	assert.Equal(t, "task-1", closedWorkstreams[0].Issue.ID)

	snapshot, err := repository.ListIssuesSnapshot(t.Context(), issue.ListRequest{
		Lifecycles: []string{"open"}, Types: []string{"task"},
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Issues, 3)
	assert.Equal(t, 3, snapshot.Total)
	assert.Equal(t, []string{"task-2", "task-3", "task-4"}, summaryIDs(snapshot.Issues))
	assert.Equal(t, int64(5), snapshot.Cursor.Revision)

	limited, err := repository.ListIssuesSnapshot(t.Context(), issue.ListRequest{
		Lifecycles: []string{"open"}, Types: []string{"task"}, Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, limited.Issues, 1)
	assert.Equal(t, 3, limited.Total)
}

func TestRepositoryListIssuesMatchesAllAnyAndNoneLabels(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	requests := []planning.CreateIssueRequest{
		{Title: "Excluded by none", Type: "task", Priority: 1, Labels: []string{"alpha", "beta", "paused"}},
		{Title: "Selected", Type: "task", Priority: 1, Labels: []string{"alpha", "gamma"}},
		{Title: "Missing all", Type: "task", Priority: 1, Labels: []string{"beta", "gamma"}},
		{Title: "Missing any", Type: "task", Priority: 1, Labels: []string{"alpha", "delta"}},
	}
	for _, request := range requests {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), request)
		require.NoError(t, err)
	}

	listed, err := repository.ListIssues(t.Context(), issue.ListRequest{
		LabelsAll:  []string{"alpha"},
		LabelsAny:  []string{"beta", "gamma"},
		LabelsNone: []string{"paused"},
	})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, "task-2", listed[0].Issue.ID)
}

func TestRepositoryListIssuesMatchesTitleRegexpAndSubstring(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	for _, title := range []string{
		"Repair parser", "Repair renderer", "Document parser repair",
	} {
		_, err := planner.CreateIssue(
			t.Context(),
			issue.NewInvocation("captain"),
			planning.CreateIssueRequest{Title: title, Type: "task", Priority: 2},
		)
		require.NoError(t, err)
	}

	regexpMatches, err := repository.ListIssues(t.Context(), issue.ListRequest{
		TitleRegexp: regexp.MustCompile(`^Repair (parser|renderer)$`),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"task-1", "task-2"}, summaryIDs(regexpMatches))

	substringMatches, err := repository.ListIssues(t.Context(), issue.ListRequest{
		TitleContains: "PARSER REPAIR",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"task-3"}, summaryIDs(substringMatches))
}

func TestRepositoryIssueContextIncludesDependencyReferenceAndResult(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "task-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	executor := execution.NewExecutor(repository, repository)

	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Selected approach", Type: "task", Priority: 1,
	})
	require.NoError(t, err)
	_, err = recorder.SetResult(t.Context(), issue.NewInvocation("captain"), record.SetResultRequest{
		IssueID: "task-1", Body: "Use the repository snapshot.",
	})
	require.NoError(t, err)
	_, err = executor.CloseIssues(t.Context(), issue.NewInvocation("captain"), execution.CloseIssuesRequest{
		IDs: []string{"task-1"},
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Implement approach", Type: "task", Priority: 2,
		DependsOn: []string{"task-1"},
	})
	require.NoError(t, err)

	depth := 0
	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{
		IssueID: "task-2", ContextDepth: &depth,
	})
	require.NoError(t, err)
	require.NotNil(t, view.Context)
	assert.Equal(t, []issue.DependencyResult{{
		Issue: issue.Reference{
			ID: "task-1", Title: "Selected approach", Type: "task",
			Status: "closed", Priority: 1,
		},
		Body: "Use the repository snapshot.",
	}}, view.Context.DependencyResults)
}

type countingQueryScope struct {
	queryScope
	calls int
}

func (s *countingQueryScope) QueryContext(
	ctx context.Context,
	query string,
	args ...any,
) (*sql.Rows, error) {
	s.calls++
	return s.queryScope.QueryContext(ctx, query, args...)
}

func (s *countingQueryScope) QueryRowContext(
	ctx context.Context,
	query string,
	args ...any,
) *sql.Row {
	s.calls++
	return s.queryScope.QueryRowContext(ctx, query, args...)
}

func mustBoardID(t *testing.T, value string) domainboard.ID {
	t.Helper()
	id, err := domainboard.NewID(value)
	require.NoError(t, err)
	return id
}

func openBoardRepository(t *testing.T, cfg Config) *Repository {
	t.Helper()
	return openBoardRepositories(t, cfg, 1)[0]
}

func openBoardRepositories(t *testing.T, cfg Config, count int) []*Repository {
	t.Helper()
	require.Positive(t, count)

	path := filepath.Join(t.TempDir(), "board.sqlite3")
	firstStore, err := store.Open(t.Context(), store.Config{
		Path: path,
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, firstStore.Close()) })

	change, err := firstStore.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at)
		VALUES ('project-test', 'Test project', 1700000000)
	`)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-test', 'project-test', 'Test board', 1700000000)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	cfg.Clock = fixedClock{now: time.Unix(1_700_000_000, 0).UTC()}
	repositories := make([]*Repository, 0, count)
	repository, err := New(firstStore, cfg)
	require.NoError(t, err)
	repositories = append(repositories, repository)
	for range count - 1 {
		persistence, err := store.OpenExisting(t.Context(), store.Config{Path: path})
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, persistence.Close()) })
		repository, err := New(persistence, cfg)
		require.NoError(t, err)
		repositories = append(repositories, repository)
	}
	return repositories
}

// fixedClock keeps operation timestamps deterministic in repository tests.
type fixedClock struct {
	// now is the timestamp returned for every operation.
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
