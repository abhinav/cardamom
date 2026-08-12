package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/execution"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.uber.org/mock/gomock"
)

func TestClaimCommand_directIssueIncludesActorAndContext(t *testing.T) {
	view := testIssueView("an-routine", "routine", "in_progress")
	view.Detail.Issue.State = new("Preserve the diagnostic position.")
	view.Detail.Issue.NextAction = new("Inspect the secondary relay.")
	view.Context.Pins = []issue.PinnedIssue{{ID: "an-pin", Title: "Pinned issue"}}
	request := execution.ClaimIssueRequest{
		ID: "an-routine", Assignee: "worker-a", ContextDepth: new(0),
	}
	result := execution.ClaimIssueResult{Issue: view}
	operations := NewMockClaimOperations(gomock.NewController(t))
	operations.EXPECT().ClaimIssue(
		gomock.Any(),
		issue.NewInvocation("worker-a"),
		request,
	).Return(result, nil)
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*ClaimOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
		"--actor", "worker-a", "--json", "claim", "an-routine", "--context",
	}))
	assert.JSONEq(t, `{
		"board": {"description": "Shared context"},
		"context": [],
		"dependency_results": [],
		"pins": [{"id":"an-pin","title":"Pinned issue"}],
		"issue": {
			"id": "an-routine",
			"title": "Test issue",
			"type": "routine",
			"lifecycle": "open",
			"status": "in_progress",
			"priority": 2,
			"active_claim": null,
			"created": 0,
			"updated": 0,
			"revision": 7,
			"keys": [],
			"labels": ["area:cli"],
			"depends_on": [],
			"blocks": [],
			"summary": "Issue summary",
			"state": "Preserve the diagnostic position.",
			"next_action": "Inspect the secondary relay.",
			"log_count": 0,
			"parent_id": null,
			"blocked": false
		}
	}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestFormatIssueViewSingleLinesPinnedTitles(t *testing.T) {
	view := testIssueView("an-task", "task", "ready")
	view.Context.Pins = []issue.PinnedIssue{{
		ID: "an-pin", Title: "Pinned\nissue\tname",
	}}

	assert.Contains(
		t,
		formatIssueView(view),
		"Pinned issues:\n- an-pin: Pinned issue name\n",
	)
}

func TestClaimCommand_automaticSelectionPassesPoolFilters(t *testing.T) {
	request := execution.ClaimNextRequest{
		UnderID:    "an-parent",
		Assignee:   "worker-b",
		LabelsAll:  []string{"area:cli", "ready"},
		LabelsAny:  []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"},
		Watch:      true,
	}
	result := execution.ClaimIssueResult{Issue: testIssueView("an-next", "task", "in_progress")}
	operations := NewMockClaimOperations(gomock.NewController(t))
	operations.EXPECT().ClaimNext(
		gomock.Any(),
		issue.NewInvocation("worker-b"),
		request,
	).Return(result, nil)
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*ClaimOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
		"--actor", "worker-b", "claim",
		"--under", "an-parent",
		"--label", "+area:cli", "--label", "ready", "--label", "-paused",
		"--label-any", "phase:a", "--label-any", "phase:b", "--watch",
	}))
	assert.Contains(t, stdout.String(), "Claimed an-next as worker-b.\n")
	assert.Contains(t, stdout.String(), "Issue an-next\n")
	assert.Contains(t, stdout.String(), "Summary\nIssue summary\n")
	assert.Empty(t, stderr.String())
}

func TestClaimCommand_directIssueRejectsAutomaticSelectionFilters(t *testing.T) {
	tests := []struct {
		name string
		give []string
	}{
		{name: "RequiredLabel", give: []string{"--label", "area:cli"}},
		{name: "ExcludedLabel", give: []string{"--label", "-paused"}},
		{name: "AlternativeLabel", give: []string{"--label-any", "phase:a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operations := NewMockClaimOperations(gomock.NewController(t))
			stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
				kong.BindTo(operations, (*ClaimOperations)(nil)),
			)
			args := append([]string{"claim", "an-1"}, tt.give...)

			assert.Equal(t, ExitUsage, app.Run(t.Context(), args))
			assert.Empty(t, stdout.String())
			assert.Equal(t, "error: --under, --label, --label-any, and --watch require automatic claim selection without an ID\n", stderr.String())
		})
	}
}

func TestClaimCommand_watchCancellationReturnsCanceledExit(t *testing.T) {
	operations := cancelingClaimOperations{}
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*ClaimOperations)(nil)),
	)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Equal(t, ExitCanceled, app.Run(ctx, []string{"claim", "--watch"}))
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestLifecycleCommandsRenderDeterministicResultsAndParentNotices(t *testing.T) {
	request := execution.CloseIssuesRequest{IDs: []string{"an-2", "an-1"}}
	result := execution.CloseIssuesResult{
		Issues: []issue.Summary{
			{Issue: issue.Issue{ID: "an-2", Title: "Second", Lifecycle: "closed", Status: "closed"}},
			{Issue: issue.Issue{ID: "an-1", Title: "First", Lifecycle: "closed", Status: "closed"}},
		},
		ParentsWithoutOpenChildren: []string{"an-parent-b", "an-parent-a"},
	}
	operations := NewMockCloseOperations(gomock.NewController(t))
	operations.EXPECT().CloseIssues(
		gomock.Any(),
		issue.NewInvocation("tester"),
		request,
	).Return(result, nil)
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*CloseOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"close", "an-2", "an-1"}))
	assert.Equal(t, strings.Join([]string{
		"Closed an-2.",
		"Closed an-1.",
		"Parent an-parent-b has no open children.",
		"Parent an-parent-a has no open children.",
		"",
	}, "\n"), stdout.String())
	assert.Empty(t, stderr.String())
}

func TestLifecycleCommandsUseNarrowDomainOperations(t *testing.T) {
	t.Run("Release", func(t *testing.T) {
		request := execution.ReleaseIssueRequest{ID: "an-1"}
		result := execution.ReleaseIssueResult{Issue: issue.Detail{
			Issue: issue.Issue{ID: "an-1", Title: "Released", Lifecycle: "open", Status: "ready"},
		}}
		operations := NewMockReleaseOperations(gomock.NewController(t))
		operations.EXPECT().ReleaseIssue(
			gomock.Any(),
			issue.NewInvocation("owner"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReleaseOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "owner", "--json", "release", "an-1",
		}))
		assert.Contains(t, stdout.String(), `"id":"an-1"`)
		assert.NotContains(t, stdout.String(), "Released an-1.")
		assert.Empty(t, stderr.String())
	})

	t.Run("ReleaseWaiting", func(t *testing.T) {
		reason := "Awaiting review"
		request := execution.ReleaseIssueRequest{ID: "an-1", WaitingReason: &reason}
		result := execution.ReleaseIssueResult{Issue: issue.Detail{
			Issue: issue.Issue{
				ID: "an-1", Lifecycle: "open", Status: "waiting",
				Waiting: &issue.Waiting{Reason: reason, Since: 10},
			},
		}}
		operations := NewMockReleaseOperations(gomock.NewController(t))
		operations.EXPECT().ReleaseIssue(
			gomock.Any(),
			issue.NewInvocation("owner"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReleaseOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "owner", "release", "an-1", "--waiting", reason,
		}))
		assert.Equal(t, "Released an-1 as waiting.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Cancel", func(t *testing.T) {
		request := execution.CancelIssuesRequest{Roots: []string{"an-root"}}
		result := execution.CancelIssuesResult{
			Issues:    []issue.Issue{{ID: "an-root"}, {ID: "an-dependent"}},
			Requested: 1, Dependents: 1,
			ParentsWithoutOpenChildren: []string{"an-parent"},
		}
		operations := NewMockCancelOperations(gomock.NewController(t))
		operations.EXPECT().CancelIssues(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*CancelOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"cancel", "an-root"}))
		assert.Equal(t, strings.Join([]string{
			"Cancelled 1 requested issue and 1 dependent issue.",
			"Parent an-parent has no open children.",
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Reopen", func(t *testing.T) {
		request := execution.ReopenIssuesRequest{IDs: []string{"an-1"}}
		result := execution.ReopenIssuesResult{Issues: []execution.ReopenedIssue{{
			Issue:                   issue.Summary{Issue: issue.Issue{ID: "an-1", Status: "ready"}},
			UnresolvedPrerequisites: []execution.PrerequisiteView{{ID: "an-blocker", Status: "ready"}},
		}}}
		operations := NewMockReopenOperations(gomock.NewController(t))
		operations.EXPECT().ReopenIssues(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReopenOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"reopen", "an-1"}))
		assert.Equal(t, strings.Join([]string{
			"Reopened an-1.",
			"an-1 remains blocked by an-blocker (ready).",
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestCheckpointCommandsMapPublicVerbsAndPreserveReasonMarkdown(t *testing.T) {
	t.Run("Approve", func(t *testing.T) {
		markdown := "First paragraph.\n\nSecond paragraph.\n"
		request := execution.CheckpointRequest{IssueID: "an-gate", Reason: markdown}
		result := execution.ResolveCheckpointResult{
			Decision: issue.CheckpointDecisionView{Outcome: "approved", Reason: markdown},
			Issue:    &issue.Issue{ID: "an-gate", Lifecycle: "closed", Status: "closed"},
		}
		operations := NewMockCheckpointOperations(gomock.NewController(t))
		operations.EXPECT().ApproveCheckpoint(
			gomock.Any(),
			issue.NewInvocation("approver"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(markdown), false,
			kong.BindTo(operations, (*CheckpointOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "approver", "checkpoint", "approve", "an-gate",
		}))
		assert.Equal(t, "Approved an-gate.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Deny", func(t *testing.T) {
		request := execution.CheckpointRequest{IssueID: "an-gate", Reason: "Not ready"}
		result := execution.ResolveCheckpointResult{
			Decision:  issue.CheckpointDecisionView{Outcome: "denied", Reason: "Not ready"},
			Cancelled: []issue.Issue{{ID: "an-gate"}, {ID: "an-dependent"}},
		}
		operations := NewMockCheckpointOperations(gomock.NewController(t))
		operations.EXPECT().DenyCheckpoint(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*CheckpointOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"checkpoint", "deny", "an-gate", "--reason", "Not ready",
		}))
		assert.Equal(t, "Denied an-gate; cancelled 1 dependent issue.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestLogEntryCommandsSelectMarkdownAndEmitJSONLines(t *testing.T) {
	t.Run("PostFromDash", func(t *testing.T) {
		request := record.AddLogEntryRequest{IssueID: "an-1", Body: "one\n\ntwo\n"}
		result := record.AddLogEntryResult{LogEntry: issue.LogEntry{
			ID: "log_41414141414141414141414141414141", IssueID: "an-1",
			Kind: "post", Author: new("worker"), Committer: new("worker"),
			Body: "one\n\ntwo\n", Created: new(int64(10)),
		}}
		operations := NewMockLogEntryWriteOperations(gomock.NewController(t))
		operations.EXPECT().AddLogEntry(
			gomock.Any(),
			issue.NewInvocation("worker"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("one\n\ntwo\n"), true,
			kong.BindTo(operations, (*LogEntryWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "worker", "--json", "log", "post", "an-1", "-",
		}))
		assert.JSONEq(t, `{
			"id": "log_41414141414141414141414141414141",
			"issue_id": "an-1",
			"kind": "post",
			"author": "worker",
			"committer": "worker",
			"body": "one\n\ntwo\n",
			"created": 10
		}`, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Show", func(t *testing.T) {
		request := issue.LogListRequest{IssueID: "an-1", Reverse: true, Limit: 2}
		result := []issue.LogEntry{
			{
				ID: "cmt_22222222222222222222222222222222", IssueID: "an-1",
				Kind: "post", Author: new("a"), Committer: new("a"),
				Body: "first", Created: new(int64(10)),
			},
			{
				ID: "cmt_33333333333333333333333333333333", IssueID: "an-1",
				Kind: "state_snapshot", Author: new("b"),
				Committer: new("c"), Body: "second", Created: new(int64(20)),
			},
		}
		operations := NewMockLogEntryReadOperations(gomock.NewController(t))
		operations.EXPECT().ListLogEntries(gomock.Any(), request).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*LogEntryReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--json", "log", "show", "an-1", "--limit", "2",
		}))
		assert.Equal(t, strings.Join([]string{
			`{"id":"cmt_22222222222222222222222222222222","issue_id":"an-1","kind":"post","author":"a","committer":"a","body":"first","created":10}`,
			`{"id":"cmt_33333333333333333333333333333333","issue_id":"an-1","kind":"state_snapshot","author":"b","committer":"c","body":"second","created":20}`,
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowOldestFirstHuman", func(t *testing.T) {
		request := issue.LogListRequest{IssueID: "an-1"}
		result := []issue.LogEntry{
			{
				ID: "cmt_22222222222222222222222222222222", IssueID: "an-1",
				Kind: "post", Author: new("a"), Committer: new("a"),
				Body: "first", Created: new(int64(10)),
			},
			{
				ID: "cmt_33333333333333333333333333333333", IssueID: "an-1",
				Kind: "state_snapshot", Author: new("b"),
				Committer: new("c"), Body: "second", Created: new(int64(20)),
			},
		}
		operations := NewMockLogEntryReadOperations(gomock.NewController(t))
		operations.EXPECT().ListLogEntries(gomock.Any(), request).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*LogEntryReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"log", "show", "an-1", "--oldest-first",
		}))
		assert.Equal(t, strings.Join([]string{
			"Post cmt_22222222222222222222222222222222 by a",
			"first",
			"",
			"State snapshot cmt_33333333333333333333333333333333 by b committed by c",
			"second",
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestStateCommandsKeepMutableStateSeparateFromLogEntries(t *testing.T) {
	updated := time.Unix(20, 0).UTC()
	snapshotID := issue.LogID("log_44444444444444444444444444444444")
	stateResult := record.StateResult{Issue: issue.Issue{ID: "an-1", State: new("current\n\nstate\n")}}
	getResult := record.GetStateResult{
		IssueID: "an-1",
		State: &issue.RecoveryState{
			Body: "current\n\nstate\n", Author: "worker",
			UpdatedAt: new(updated), SnapshotLogEntryID: &snapshotID,
		},
	}
	commitResult := record.CommitStateResult{
		Issue: issue.Issue{ID: "an-1"},
		State: &issue.RecoveryState{
			Body:       "next\n\n- inspect coils\n",
			NextAction: "Report coil status.", Author: "worker",
			UpdatedAt: new(updated),
		},
		LogEntry: &issue.LogEntry{
			ID: "log_55555555555555555555555555555555", IssueID: "an-1",
			Kind: "state_snapshot", Author: new("author"),
			Committer: new("worker"), Body: "current", Created: new(int64(20)),
		},
	}

	t.Run("SetFromPipe", func(t *testing.T) {
		request := record.SetStateRequest{IssueID: "an-1", Text: "current\n\nstate\n"}
		operations := NewMockStateWriteOperations(gomock.NewController(t))
		operations.EXPECT().SetState(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(stateResult, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("current\n\nstate\n"), false,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "set", "an-1"}))
		assert.Equal(t, "Set state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowJSON", func(t *testing.T) {
		request := record.GetStateRequest{IssueID: "an-1"}
		operations := NewMockStateReadOperations(gomock.NewController(t))
		operations.EXPECT().GetState(gomock.Any(), request).Return(getResult, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"--json", "state", "show", "an-1"}))
		assert.JSONEq(t, `{
			"issue_id": "an-1",
			"body": "current\n\nstate\n",
			"author": "worker",
			"updated": 20,
			"snapshot_log_entry_id": "log_44444444444444444444444444444444"
		}`, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("AppendArgument", func(t *testing.T) {
		request := record.SetStateRequest{IssueID: "an-1", Text: "next"}
		operations := NewMockStateWriteOperations(gomock.NewController(t))
		operations.EXPECT().AppendState(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(stateResult, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("ignored"), true,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "append", "an-1", "next"}))
		assert.Equal(t, "Appended state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("SetEmpty", func(t *testing.T) {
		request := record.SetStateRequest{IssueID: "an-1"}
		operations := NewMockStateWriteOperations(gomock.NewController(t))
		operations.EXPECT().SetState(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(stateResult, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "set", "an-1", ""}))
		assert.Equal(t, "Set state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("CommitDefaultClear", func(t *testing.T) {
		request := record.CommitStateRequest{
			IssueID: "an-1", Disposition: record.CommitStateClear,
		}
		result := record.CommitStateResult{Issue: issue.Issue{ID: "an-1"}}
		operations := NewMockStateCommitOperations(gomock.NewController(t))
		operations.EXPECT().CommitState(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateCommitOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"state", "commit", "an-1",
		}))
		assert.Equal(t, "Committed state on an-1; no new snapshot.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("CommitSetFromDash", func(t *testing.T) {
		request := record.CommitStateRequest{
			IssueID: "an-1", Disposition: record.CommitStateReplace,
			Replacement: record.StateReplacement{
				Body:       "next\n\n- inspect coils\n",
				NextAction: "Report coil status.",
			},
		}
		operations := NewMockStateCommitOperations(gomock.NewController(t))
		operations.EXPECT().CommitState(
			gomock.Any(),
			issue.NewInvocation("worker"),
			request,
		).Return(commitResult, nil)
		stdout, stderr, app := executionTestApplication(
			t,
			strings.NewReader("next\n\n- inspect coils\n"),
			true,
			kong.BindTo(operations, (*StateCommitOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "worker", "--json", "state", "commit", "an-1",
			"--set", "-", "--next", "Report coil status.",
		}))
		assert.JSONEq(t, `{
			"issue_id": "an-1",
			"snapshot_created": true,
				"state": {
					"body": "next\n\n- inspect coils\n",
					"next_action": "Report coil status.",
				"author": "worker",
				"updated": 20
			},
			"log_entry": {
				"id": "log_55555555555555555555555555555555",
				"issue_id": "an-1",
				"kind": "state_snapshot",
				"author": "author",
				"committer": "worker",
				"body": "current",
				"created": 20
			}
		}`, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("CommitEmptySet", func(t *testing.T) {
		request := record.CommitStateRequest{
			IssueID: "an-1", Disposition: record.CommitStateClear,
		}
		result := record.CommitStateResult{Issue: issue.Issue{ID: "an-1"}}
		operations := NewMockStateCommitOperations(gomock.NewController(t))
		operations.EXPECT().CommitState(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateCommitOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"state", "commit", "an-1", "--set", "",
		}))
		assert.Equal(t, "Committed state on an-1; no new snapshot.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestResultCommandsKeepResultSeparateAndRequireDomainValidation(t *testing.T) {
	t.Run("SetArgument", func(t *testing.T) {
		request := record.SetResultRequest{IssueID: "an-1", Body: "Outcome\n"}
		result := record.SetResultResult{IssueID: "an-1", Body: "Outcome\n"}
		operations := NewMockResultWriteOperations(gomock.NewController(t))
		operations.EXPECT().SetResult(
			gomock.Any(),
			issue.NewInvocation("tester"),
			request,
		).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("ignored"), true,
			kong.BindTo(operations, (*ResultWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"result", "set", "an-1", "Outcome\n"}))
		assert.Equal(t, "Set result on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowJSON", func(t *testing.T) {
		request := record.GetResultRequest{IssueID: "an-1"}
		result := issue.Result{IssueID: "an-1", Title: "Decision", Body: "Outcome\n"}
		operations := NewMockResultReadOperations(gomock.NewController(t))
		operations.EXPECT().GetResult(gomock.Any(), request).Return(result, nil)
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ResultReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"--json", "result", "show", "an-1"}))
		assert.JSONEq(t, `{"issue_id":"an-1","title":"Decision","body":"Outcome\n"}`, stdout.String())
		assert.Empty(t, stderr.String())
	})
}

type cancelingClaimOperations struct{}

func (cancelingClaimOperations) ClaimIssue(context.Context, issue.Invocation, execution.ClaimIssueRequest) (execution.ClaimIssueResult, error) {
	return execution.ClaimIssueResult{}, assert.AnError
}

func (cancelingClaimOperations) ClaimNext(ctx context.Context, _ issue.Invocation, _ execution.ClaimNextRequest) (execution.ClaimIssueResult, error) {
	return execution.ClaimIssueResult{}, ctx.Err()
}

func executionTestApplication(t *testing.T, stdin *strings.Reader, stdinIsTerminal bool, options ...kong.Option) (*bytes.Buffer, *bytes.Buffer, *Application) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	config := testConfig(&stdout, &stderr)
	config.Stdin = stdin
	config.StdinIsTerminal = stdinIsTerminal
	app, err := New(config, options...)
	require.NoError(t, err)
	return &stdout, &stderr, app
}

func testIssueView(id, kind, status string) issue.View {
	summary := "Issue summary"
	boardDescription := "Shared context"
	return issue.View{
		Detail: issue.Detail{
			Issue: issue.Issue{
				ID: id, Title: "Test issue", Type: kind, Lifecycle: "open", Status: status,
				Priority: 2, Summary: &summary, Revision: 7,
			},
			Labels: []string{"area:cli"},
		},
		Context: &issue.Context{
			Board: issue.BoardDescription{Description: &boardDescription},
		},
	}
}
