package board

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/planning"
	"go.abhg.dev/cardamom/internal/issue/record"
)

func TestRepositoryApplyDocumentDryRunAndFailureDoNotConsumeIDs(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	cyclic := applyRequest(
		applyIssue("first", "First", "task"),
		applyIssue("second", "Second", "task"),
	)
	cyclic.Issues[0].DependsOn = applyReferences(aliasReference("second"))
	cyclic.Issues[1].DependsOn = applyReferences(aliasReference("first"))
	_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), cyclic)
	require.ErrorContains(t, err, "dependency graph must remain acyclic")

	valid := applyRequest(
		applyIssue("child", "Child", "task"),
		applyIssue("parent", "Parent", "workstream"),
	)
	valid.Issues[0].DependsOn = applyReferences(aliasReference("parent"))
	valid.Issues[0].Parent = planning.ApplyParentChange{
		Kind: planning.ParentReplace, Reference: aliasReference("parent"),
	}
	valid.Mode = planning.ApplyModeDryRun
	dryRun, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), valid)
	require.NoError(t, err)
	assert.True(t, dryRun.DryRun)
	assert.Equal(t, planning.ApplyCounts{Create: 2}, dryRun.Counts)
	assert.Nil(t, dryRun.Entries[0].ID)
	assert.Nil(t, dryRun.Revision)

	valid.Mode = planning.ApplyModeCommit
	committed, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), valid)
	require.NoError(t, err)
	assert.False(t, committed.DryRun)
	require.NotNil(t, committed.Entries[0].ID)
	assert.Equal(t, "graph-1", *committed.Entries[0].ID)
	require.NotNil(t, committed.Entries[1].ID)
	assert.Equal(t, "graph-2", *committed.Entries[1].ID)
	require.NotNil(t, committed.Revision)
}

func TestRepositoryApplyDocumentUsesPresenceAndReplacesRelationships(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	initial := applyRequest(
		applyIssue("first-parent", "First parent", "workstream"),
		applyIssue("second-parent", "Second parent", "workstream"),
		applyIssue("first-dependency", "First dependency", "task"),
		applyIssue("second-dependency", "Second dependency", "task"),
		applyIssue("target", "Target", "task"),
	)
	initial.Issues[4].Key = new("source:target")
	initial.Issues[4].Summary = new("Original summary.")
	initial.Issues[4].Labels = new([]string{"first"})
	initial.Issues[4].DependsOn = applyReferences(aliasReference("first-dependency"))
	initial.Issues[4].Parent = planning.ApplyParentChange{
		Kind: planning.ParentReplace, Reference: aliasReference("first-parent"),
	}
	_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), initial)
	require.NoError(t, err)

	update := applyRequest(planning.ApplyIssue{
		Key: new("source:target"), Title: new("Renamed"),
		Labels:    new([]string{"second"}),
		DependsOn: applyReferences(idReference("graph-4")),
		Parent: planning.ApplyParentChange{
			Kind: planning.ParentReplace, Reference: idReference("graph-2"),
		},
	})
	update.OnExisting = planning.ApplyExistingUpdate
	receipt, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), update)
	require.NoError(t, err)
	assert.Equal(t, planning.ApplyCounts{Update: 1}, receipt.Counts)
	assert.Equal(t, planning.ApplyActionUpdate, receipt.Entries[0].Action)
	require.NotNil(t, receipt.Entries[0].ID)
	assert.Equal(t, "graph-5", *receipt.Entries[0].ID)
	require.NotNil(t, receipt.Entries[0].Key)
	assert.Equal(t, "source:target", *receipt.Entries[0].Key)

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "graph-5"})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", view.Detail.Issue.Title)
	assert.Equal(t, new("Original summary."), view.Detail.Issue.Summary)
	assert.Equal(t, []string{"second"}, view.Detail.Labels)
	assert.Equal(t, new("graph-2"), view.Detail.ParentID)
	require.Len(t, view.Detail.DependsOn, 1)
	assert.Equal(t, "graph-4", view.Detail.DependsOn[0].ID)

	clearRequest := applyRequest(planning.ApplyIssue{
		Key:    new("source:target"),
		Parent: planning.ApplyParentChange{Kind: planning.ParentClear},
	})
	clearRequest.OnExisting = planning.ApplyExistingUpdate
	_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), clearRequest)
	require.NoError(t, err)
	view, err = repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "graph-5"})
	require.NoError(t, err)
	assert.Nil(t, view.Detail.ParentID)
}

func TestRepositoryApplyDocumentRejectsUnsafeUpdateTargets(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*execution.Executor) error
	}{
		{
			name: "Claimed",
			prepare: func(executor *execution.Executor) error {
				_, err := executor.ClaimIssue(
					t.Context(),
					issue.NewInvocation("worker"),
					execution.ClaimIssueRequest{ID: "graph-1", Assignee: "worker"},
				)
				return err
			},
		},
		{
			name: "Closed",
			prepare: func(executor *execution.Executor) error {
				_, err := executor.ClaimIssue(
					t.Context(),
					issue.NewInvocation("worker"),
					execution.ClaimIssueRequest{ID: "graph-1", Assignee: "worker"},
				)
				if err != nil {
					return err
				}
				_, err = executor.CloseIssues(
					t.Context(),
					issue.NewInvocation("worker"),
					execution.CloseIssuesRequest{IDs: []string{"graph-1"}},
				)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := openBoardRepository(t, Config{
				BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
			})
			planner := planning.NewPlanner(repository, repository, nil)
			executor := execution.NewExecutor(repository, repository)
			initial := applyRequest(applyIssue("target", "Target", "task"))
			initial.Issues[0].Key = new("source:target")
			_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), initial)
			require.NoError(t, err)
			require.NoError(t, test.prepare(executor))

			update := applyRequest(planning.ApplyIssue{
				Key: new("source:target"), Title: new("Renamed"),
			})
			update.OnExisting = planning.ApplyExistingUpdate
			_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), update)
			require.ErrorContains(t, err, "must be open and unclaimed")
		})
	}
}

func TestRepositoryApplyDocumentValidatesCompleteGraphsAtomically(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	initial := applyRequest(
		applyIssue("first", "First", "workstream"),
		applyIssue("second", "Second", "workstream"),
	)
	initial.Issues[0].Key = new("source:first")
	initial.Issues[1].Key = new("source:second")
	initial.Issues[1].DependsOn = applyReferences(aliasReference("first"))
	initial.Issues[1].Parent = planning.ApplyParentChange{
		Kind: planning.ParentReplace, Reference: aliasReference("first"),
	}
	_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), initial)
	require.NoError(t, err)

	dependencyCycle := applyRequest(planning.ApplyIssue{
		Key: new("source:first"), DependsOn: applyReferences(idReference("graph-2")),
	})
	dependencyCycle.OnExisting = planning.ApplyExistingUpdate
	_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), dependencyCycle)
	require.ErrorContains(t, err, "dependency graph must remain acyclic")

	containmentCycle := applyRequest(planning.ApplyIssue{
		Key: new("source:first"),
		Parent: planning.ApplyParentChange{
			Kind: planning.ParentReplace, Reference: idReference("graph-2"),
		},
	})
	containmentCycle.OnExisting = planning.ApplyExistingUpdate
	_, err = planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), containmentCycle)
	require.ErrorContains(t, err, "containment would create a cycle")

	view, err := repository.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "graph-1"})
	require.NoError(t, err)
	assert.Empty(t, view.Detail.DependsOn)
	assert.Nil(t, view.Detail.ParentID)
}

func TestRepositoryForeignOwnershipLookupUsesDocumentIssueIDs(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
	})
	otherRepository := openAdditionalBoardRepository(t, repository, "board-other", "foreign-")
	otherPlanner := planning.NewPlanner(otherRepository, otherRepository, nil)
	for _, title := range []string{"Unrelated foreign issue", "Requested foreign issue"} {
		_, err := otherPlanner.CreateIssue(t.Context(), issue.NewInvocation("other"), planning.CreateIssueRequest{
			Title: title, Type: "task", Priority: 2,
		})
		require.NoError(t, err)
	}

	view, err := repository.store.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()

	owners, err := repository.readForeignIssueBoards(t.Context(), view, []issue.ID{
		issue.MustID("foreign-2"),
		issue.MustID("missing-1"),
	})
	require.NoError(t, err)
	require.Len(t, owners, 1)
	assert.Equal(t, mustBoardID(t, "board-other"), owners[issue.MustID("foreign-2")])
	assert.NotContains(t, owners, issue.MustID("foreign-1"))
}

func TestRepositoryApplyDocumentDistinguishesForeignAndUnknownIssueIDs(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "graph-", IDStrategy: "sequential",
	})
	otherRepository := openAdditionalBoardRepository(t, repository, "board-other", "foreign-")
	otherPlanner := planning.NewPlanner(otherRepository, otherRepository, nil)
	_, err := otherPlanner.CreateIssue(t.Context(), issue.NewInvocation("other"), planning.CreateIssueRequest{
		Title: "Foreign issue", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	planner := planning.NewPlanner(repository, repository, nil)
	for _, test := range []struct {
		name      string
		id        string
		wantError string
	}{
		{
			name: "Foreign",
			id:   "foreign-1",
			wantError: "issue ID \"foreign-1\" belongs to board \"board-other\", " +
				"not selected board \"board-test\"",
		},
		{
			name:      "Unknown",
			id:        "missing-1",
			wantError: "issue not found: issue ID \"missing-1\"",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := applyRequest(planning.ApplyIssue{ID: new(test.id)})
			request.OnExisting = planning.ApplyExistingUpdate
			request.Mode = planning.ApplyModeDryRun
			_, err := planner.ApplyDocument(t.Context(), issue.NewInvocation("captain"), request)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestRepositoryReadsDumpSnapshot(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "local-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	recorder := record.NewRecorder(repository, repository)
	var err error

	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Program", Type: "workstream", Priority: 2, Labels: []string{"program"},
	})
	require.NoError(t, err)
	_, err = planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Task", Type: "task", Priority: 2, Labels: []string{"backend"},
		DependsOn: []string{"local-1"}, Parent: "local-1",
	})
	require.NoError(t, err)
	_, err = recorder.AddLogEntry(t.Context(), issue.NewInvocation("captain"), record.AddLogEntryRequest{
		IssueID: "local-2", Body: "Implementation context.",
	})
	require.NoError(t, err)
	_, err = recorder.SetResult(t.Context(), issue.NewInvocation("captain"), record.SetResultRequest{
		IssueID: "local-1", Body: "Program complete.",
	})
	require.NoError(t, err)

	created := time.Unix(1_700_000_000, 0).UTC()
	snapshot, err := repository.ReadDumpSnapshot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dump.BoardSnapshot{
		BoardID: "board-test", Revision: 4,
		Issues: []dump.Issue{
			{
				ID: "local-1", Title: "Program", Type: "workstream", Status: "ready",
				Priority: 2, Created: created.Unix(), Updated: created.Unix(),
				Revision: 4, Labels: []string{"program"},
			},
			{
				ID: "local-2", Title: "Task", Type: "task", Status: "blocked",
				Priority: 2, Created: created.Unix(), Updated: created.Unix(),
				Revision: 3, Labels: []string{"backend"},
			},
		},
		Dependencies: []dump.Dependency{{ChildID: "local-2", ParentID: "local-1"}},
		Containment:  []dump.Containment{{ChildID: "local-2", ParentID: "local-1"}},
		Results:      []dump.Result{{IssueID: "local-1", Body: "Program complete."}},
		LogEntries: []dump.LogEntry{{
			ID: snapshot.LogEntries[0].ID, IssueID: "local-2", Kind: "post",
			Author: new("captain"), Committer: new("captain"),
			Body: "Implementation context.", Created: new(created.Unix()),
		}},
	}, snapshot)
}

func TestRepositoryDumpSnapshotRetainsGlobalStoreRevision(t *testing.T) {
	repository := openBoardRepository(t, Config{
		BoardID: mustBoardID(t, "board-test"), IDPrefix: "local-", IDStrategy: "sequential",
	})
	planner := planning.NewPlanner(repository, repository, nil)
	_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
		Title: "Local issue", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	other := openAdditionalBoardRepository(t, repository, "board-other", "other-")
	otherPlanner := planning.NewPlanner(other, other, nil)
	_, err = otherPlanner.CreateIssue(t.Context(), issue.NewInvocation("engineer"), planning.CreateIssueRequest{
		Title: "Other issue", Type: "task", Priority: 2,
	})
	require.NoError(t, err)

	snapshot, err := repository.ReadDumpSnapshot(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(2), snapshot.Revision)
	cursor, err := repository.ReadChangeCursor(t.Context())
	require.NoError(t, err)
	assert.Equal(t, int64(1), cursor.Revision)
}

func applyRequest(issues ...planning.ApplyIssue) planning.ApplyDocumentRequest {
	return planning.ApplyDocumentRequest{
		Version: 1, Issues: issues, OnExisting: planning.ApplyExistingError,
		Mode: planning.ApplyModeCommit,
	}
}

func applyIssue(alias, title, issueType string) planning.ApplyIssue {
	return planning.ApplyIssue{Alias: &alias, Title: &title, Type: &issueType}
}

func applyContainedIssue(
	alias string,
	title string,
	issueType string,
	parent string,
) planning.ApplyIssue {
	value := applyIssue(alias, title, issueType)
	value.Parent = planning.ApplyParentChange{
		Kind: planning.ParentReplace, Reference: aliasReference(parent),
	}
	return value
}

func applyReferences(
	values ...planning.ApplyIssueReference,
) *[]planning.ApplyIssueReference {
	return new(values)
}

func aliasReference(value string) planning.ApplyIssueReference {
	return planning.ApplyIssueReference{Kind: planning.ApplyReferenceAlias, Alias: value}
}

func idReference(value string) planning.ApplyIssueReference {
	return planning.ApplyIssueReference{Kind: planning.ApplyReferenceID, ID: value}
}

func openAdditionalBoardRepository(
	t *testing.T,
	repository *Repository,
	boardID string,
	idPrefix string,
) *Repository {
	t.Helper()
	change, err := repository.store.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES (?, 'project-test', ?, 1700000000)
	`, boardID, boardID)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())

	other, err := New(repository.store, Config{
		BoardID: mustBoardID(t, boardID), IDPrefix: idPrefix, IDStrategy: "sequential",
		Clock: repository.clock,
	})
	require.NoError(t, err)
	return other
}
