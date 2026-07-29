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
)

func TestClaimCommand_directIssueIncludesActorAndContext(t *testing.T) {
	view := testIssueView("an-routine", "routine", "in_progress")
	view.Detail.Issue.State = new("Preserve the diagnostic position.")
	view.Detail.Issue.NextAction = new("Inspect the secondary relay.")
	operations := &claimOperationsRecorder{
		claimIssueResult: execution.ClaimIssueResult{Issue: view},
	}
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*ClaimOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
		"--actor", "worker-a", "--json", "claim", "an-routine", "--context",
	}))
	assert.Equal(t, "worker-a", operations.invocation.Actor())
	assert.Equal(t, "an-routine", operations.claimIssueRequest.ID)
	assert.Equal(t, "worker-a", operations.claimIssueRequest.Assignee)
	require.NotNil(t, operations.claimIssueRequest.ContextDepth)
	assert.Zero(t, *operations.claimIssueRequest.ContextDepth)
	assert.False(t, operations.claimNextCalled)
	assert.JSONEq(t, `{
		"board": {"description": "Shared context"},
		"context": [],
		"dependency_results": [],
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

func TestClaimCommand_automaticSelectionPassesPoolFilters(t *testing.T) {
	operations := &claimOperationsRecorder{
		claimNextResult: execution.ClaimIssueResult{Issue: testIssueView("an-next", "task", "in_progress")},
	}
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*ClaimOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
		"--actor", "worker-b", "claim",
		"--under", "an-parent",
		"--label", "+area:cli", "--label", "ready", "--label", "-paused",
		"--label-any", "phase:a", "--label-any", "phase:b", "--watch",
	}))
	assert.True(t, operations.claimNextCalled)
	assert.Equal(t, execution.ClaimNextRequest{
		UnderID:    "an-parent",
		Assignee:   "worker-b",
		LabelsAll:  []string{"area:cli", "ready"},
		LabelsAny:  []string{"phase:a", "phase:b"},
		LabelsNone: []string{"paused"},
		Watch:      true,
	}, operations.claimNextRequest)
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
			operations := &claimOperationsRecorder{}
			stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
				kong.BindTo(operations, (*ClaimOperations)(nil)),
			)
			args := append([]string{"claim", "an-1"}, tt.give...)

			assert.Equal(t, ExitUsage, app.Run(t.Context(), args))
			assert.False(t, operations.claimNextCalled)
			assert.Empty(t, operations.claimIssueRequest.ID)
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
	operations := &lifecycleOperationsRecorder{
		closeResult: execution.CloseIssuesResult{
			Issues: []issue.Summary{
				{Issue: issue.Issue{ID: "an-2", Title: "Second", Lifecycle: "closed", Status: "closed"}},
				{Issue: issue.Issue{ID: "an-1", Title: "First", Lifecycle: "closed", Status: "closed"}},
			},
			ParentsWithoutOpenChildren: []string{"an-parent-b", "an-parent-a"},
		},
	}
	stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
		kong.BindTo(operations, (*CloseOperations)(nil)),
	)

	assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"close", "an-2", "an-1"}))
	assert.Equal(t, []string{"an-2", "an-1"}, operations.closeRequest.IDs)
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
		operations := &lifecycleOperationsRecorder{
			releaseResult: execution.ReleaseIssueResult{Issue: issue.Detail{
				Issue: issue.Issue{ID: "an-1", Title: "Released", Lifecycle: "open", Status: "ready"},
			}},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReleaseOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "owner", "--json", "release", "an-1",
		}))
		assert.Equal(t, "owner", operations.invocation.Actor())
		assert.Equal(t, execution.ReleaseIssueRequest{ID: "an-1"}, operations.releaseRequest)
		assert.Contains(t, stdout.String(), `"id":"an-1"`)
		assert.NotContains(t, stdout.String(), "Released an-1.")
		assert.Empty(t, stderr.String())
	})

	t.Run("ReleaseWaiting", func(t *testing.T) {
		reason := "Awaiting review"
		operations := &lifecycleOperationsRecorder{
			releaseResult: execution.ReleaseIssueResult{Issue: issue.Detail{
				Issue: issue.Issue{
					ID: "an-1", Lifecycle: "open", Status: "waiting",
					Waiting: &issue.Waiting{Reason: reason, Since: 10},
				},
			}},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReleaseOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "owner", "release", "an-1", "--waiting", reason,
		}))
		assert.Equal(t, execution.ReleaseIssueRequest{ID: "an-1", WaitingReason: &reason}, operations.releaseRequest)
		assert.Equal(t, "Released an-1 as waiting.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Cancel", func(t *testing.T) {
		operations := &lifecycleOperationsRecorder{
			cancelResult: execution.CancelIssuesResult{
				Issues:    []issue.Issue{{ID: "an-root"}, {ID: "an-dependent"}},
				Requested: 1, Dependents: 1,
				ParentsWithoutOpenChildren: []string{"an-parent"},
			},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*CancelOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"cancel", "an-root"}))
		assert.Equal(t, execution.CancelIssuesRequest{Roots: []string{"an-root"}}, operations.cancelRequest)
		assert.Equal(t, strings.Join([]string{
			"Cancelled 1 requested issue and 1 dependent issue.",
			"Parent an-parent has no open children.",
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Reopen", func(t *testing.T) {
		operations := &lifecycleOperationsRecorder{
			reopenResult: execution.ReopenIssuesResult{Issues: []execution.ReopenedIssue{{
				Issue:                   issue.Summary{Issue: issue.Issue{ID: "an-1", Status: "ready"}},
				UnresolvedPrerequisites: []execution.PrerequisiteView{{ID: "an-blocker", Status: "ready"}},
			}}},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ReopenOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"reopen", "an-1"}))
		assert.Equal(t, execution.ReopenIssuesRequest{IDs: []string{"an-1"}}, operations.reopenRequest)
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
		operations := &checkpointOperationsRecorder{
			approveResult: execution.ResolveCheckpointResult{
				Decision: issue.CheckpointDecisionView{Outcome: "approved", Reason: markdown},
				Issue:    &issue.Issue{ID: "an-gate", Lifecycle: "closed", Status: "closed"},
			},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(markdown), false,
			kong.BindTo(operations, (*CheckpointOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "approver", "checkpoint", "approve", "an-gate",
		}))
		assert.True(t, operations.approveCalled)
		assert.False(t, operations.denyCalled)
		assert.Equal(t, "approver", operations.invocation.Actor())
		assert.Equal(t, execution.CheckpointRequest{IssueID: "an-gate", Reason: markdown}, operations.request)
		assert.Equal(t, "Approved an-gate.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Deny", func(t *testing.T) {
		operations := &checkpointOperationsRecorder{
			denyResult: execution.ResolveCheckpointResult{
				Decision:  issue.CheckpointDecisionView{Outcome: "denied", Reason: "Not ready"},
				Cancelled: []issue.Issue{{ID: "an-gate"}, {ID: "an-dependent"}},
			},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*CheckpointOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"checkpoint", "deny", "an-gate", "--reason", "Not ready",
		}))
		assert.False(t, operations.approveCalled)
		assert.True(t, operations.denyCalled)
		assert.Equal(t, execution.CheckpointRequest{IssueID: "an-gate", Reason: "Not ready"}, operations.request)
		assert.Equal(t, "Denied an-gate; cancelled 1 dependent issue.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestLogEntryCommandsSelectMarkdownAndEmitJSONLines(t *testing.T) {
	t.Run("PostFromDash", func(t *testing.T) {
		operations := &logOperationsRecorder{
			addResult: record.AddLogEntryResult{LogEntry: issue.LogEntry{
				ID: "log_41414141414141414141414141414141", IssueID: "an-1",
				Kind: "post", Author: new("worker"), Committer: new("worker"),
				Body: "one\n\ntwo\n", Created: new(int64(10)),
			}},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("one\n\ntwo\n"), true,
			kong.BindTo(operations, (*LogEntryWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--actor", "worker", "--json", "log", "post", "an-1", "-",
		}))
		assert.Equal(t, "worker", operations.invocation.Actor())
		assert.Equal(t, record.AddLogEntryRequest{IssueID: "an-1", Body: "one\n\ntwo\n"}, operations.addRequest)
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
		operations := &logOperationsRecorder{
			entries: []issue.LogEntry{
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
			},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*LogEntryReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--json", "log", "show", "an-1", "--limit", "2",
		}))
		assert.Equal(t, issue.LogListRequest{IssueID: "an-1", Reverse: true, Limit: 2}, operations.listRequest)
		assert.Equal(t, strings.Join([]string{
			`{"id":"cmt_22222222222222222222222222222222","issue_id":"an-1","kind":"post","author":"a","committer":"a","body":"first","created":10}`,
			`{"id":"cmt_33333333333333333333333333333333","issue_id":"an-1","kind":"state_snapshot","author":"b","committer":"c","body":"second","created":20}`,
			"",
		}, "\n"), stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowOldestFirstHuman", func(t *testing.T) {
		operations := &logOperationsRecorder{
			entries: []issue.LogEntry{
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
			},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*LogEntryReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"log", "show", "an-1", "--oldest-first",
		}))
		assert.Equal(t, issue.LogListRequest{IssueID: "an-1"}, operations.listRequest)
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
	operations := &stateOperationsRecorder{
		stateResult: record.StateResult{Issue: issue.Issue{ID: "an-1", State: new("current\n\nstate\n")}},
		getResult: record.GetStateResult{
			IssueID: "an-1",
			State: &issue.RecoveryState{
				Body: "current\n\nstate\n", Author: "worker",
				UpdatedAt: new(updated), SnapshotLogEntryID: &snapshotID,
			},
		},
		commitResult: record.CommitStateResult{
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
		},
	}

	t.Run("SetFromPipe", func(t *testing.T) {
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("current\n\nstate\n"), false,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "set", "an-1"}))
		assert.Equal(t, record.SetStateRequest{IssueID: "an-1", Text: "current\n\nstate\n"}, operations.setRequest)
		assert.Equal(t, "Set state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowJSON", func(t *testing.T) {
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"--json", "state", "show", "an-1"}))
		assert.Equal(t, record.GetStateRequest{IssueID: "an-1"}, operations.readRequest)
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
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("ignored"), true,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "append", "an-1", "next"}))
		assert.Equal(t, record.SetStateRequest{IssueID: "an-1", Text: "next"}, operations.appendRequest)
		assert.Equal(t, "Appended state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("SetEmpty", func(t *testing.T) {
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"state", "set", "an-1", ""}))
		assert.Equal(t, record.SetStateRequest{IssueID: "an-1"}, operations.setRequest)
		assert.Equal(t, "Set state on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("CommitSetFromDash", func(t *testing.T) {
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
		assert.Equal(t, record.CommitStateRequest{
			IssueID: "an-1", Disposition: record.CommitStateReplace,
			Replacement: record.StateReplacement{
				Body:       "next\n\n- inspect coils\n",
				NextAction: "Report coil status.",
			},
		}, operations.commitRequest)
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
		operations.commitResult = record.CommitStateResult{
			Issue: issue.Issue{ID: "an-1"},
		}
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*StateCommitOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"state", "commit", "an-1", "--set", "",
		}))
		assert.Equal(t, record.CommitStateRequest{
			IssueID: "an-1", Disposition: record.CommitStateClear,
		}, operations.commitRequest)
		assert.Equal(t, "Committed state on an-1; no new snapshot.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestResultCommandsKeepResultSeparateAndRequireDomainValidation(t *testing.T) {
	operations := &resultOperationsRecorder{
		setResult:   record.SetResultResult{IssueID: "an-1", Body: "Outcome\n"},
		issueResult: issue.Result{IssueID: "an-1", Title: "Decision", Body: "Outcome\n"},
	}

	t.Run("SetArgument", func(t *testing.T) {
		stdout, stderr, app := executionTestApplication(t, strings.NewReader("ignored"), true,
			kong.BindTo(operations, (*ResultWriteOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"result", "set", "an-1", "Outcome\n"}))
		assert.Equal(t, record.SetResultRequest{IssueID: "an-1", Body: "Outcome\n"}, operations.setRequest)
		assert.Equal(t, "Set result on an-1.\n", stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("ShowJSON", func(t *testing.T) {
		stdout, stderr, app := executionTestApplication(t, strings.NewReader(""), true,
			kong.BindTo(operations, (*ResultReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{"--json", "result", "show", "an-1"}))
		assert.Equal(t, record.GetResultRequest{IssueID: "an-1"}, operations.readRequest)
		assert.JSONEq(t, `{"issue_id":"an-1","title":"Decision","body":"Outcome\n"}`, stdout.String())
		assert.Empty(t, stderr.String())
	})
}

type claimOperationsRecorder struct {
	invocation        issue.Invocation
	claimIssueRequest execution.ClaimIssueRequest
	claimNextRequest  execution.ClaimNextRequest
	claimIssueResult  execution.ClaimIssueResult
	claimNextResult   execution.ClaimIssueResult
	claimNextCalled   bool
}

func (o *claimOperationsRecorder) ClaimIssue(_ context.Context, inv issue.Invocation, req execution.ClaimIssueRequest) (execution.ClaimIssueResult, error) {
	o.invocation = inv
	o.claimIssueRequest = req
	return o.claimIssueResult, nil
}

func (o *claimOperationsRecorder) ClaimNext(_ context.Context, inv issue.Invocation, req execution.ClaimNextRequest) (execution.ClaimIssueResult, error) {
	o.invocation = inv
	o.claimNextRequest = req
	o.claimNextCalled = true
	return o.claimNextResult, nil
}

type cancelingClaimOperations struct{}

func (cancelingClaimOperations) ClaimIssue(context.Context, issue.Invocation, execution.ClaimIssueRequest) (execution.ClaimIssueResult, error) {
	return execution.ClaimIssueResult{}, assert.AnError
}

func (cancelingClaimOperations) ClaimNext(ctx context.Context, _ issue.Invocation, _ execution.ClaimNextRequest) (execution.ClaimIssueResult, error) {
	return execution.ClaimIssueResult{}, ctx.Err()
}

type lifecycleOperationsRecorder struct {
	invocation     issue.Invocation
	releaseRequest execution.ReleaseIssueRequest
	closeRequest   execution.CloseIssuesRequest
	cancelRequest  execution.CancelIssuesRequest
	reopenRequest  execution.ReopenIssuesRequest
	releaseResult  execution.ReleaseIssueResult
	closeResult    execution.CloseIssuesResult
	cancelResult   execution.CancelIssuesResult
	reopenResult   execution.ReopenIssuesResult
}

func (o *lifecycleOperationsRecorder) ReleaseIssue(_ context.Context, inv issue.Invocation, req execution.ReleaseIssueRequest) (execution.ReleaseIssueResult, error) {
	o.invocation = inv
	o.releaseRequest = req
	return o.releaseResult, nil
}

func (o *lifecycleOperationsRecorder) CloseIssues(_ context.Context, _ issue.Invocation, req execution.CloseIssuesRequest) (execution.CloseIssuesResult, error) {
	o.closeRequest = req
	return o.closeResult, nil
}

func (o *lifecycleOperationsRecorder) CancelIssues(_ context.Context, _ issue.Invocation, req execution.CancelIssuesRequest) (execution.CancelIssuesResult, error) {
	o.cancelRequest = req
	return o.cancelResult, nil
}

func (o *lifecycleOperationsRecorder) ReopenIssues(_ context.Context, _ issue.Invocation, req execution.ReopenIssuesRequest) (execution.ReopenIssuesResult, error) {
	o.reopenRequest = req
	return o.reopenResult, nil
}

type checkpointOperationsRecorder struct {
	invocation    issue.Invocation
	request       execution.CheckpointRequest
	approveResult execution.ResolveCheckpointResult
	denyResult    execution.ResolveCheckpointResult
	approveCalled bool
	denyCalled    bool
}

func (o *checkpointOperationsRecorder) ApproveCheckpoint(_ context.Context, inv issue.Invocation, req execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
	o.invocation = inv
	o.request = req
	o.approveCalled = true
	return o.approveResult, nil
}

func (o *checkpointOperationsRecorder) DenyCheckpoint(_ context.Context, inv issue.Invocation, req execution.CheckpointRequest) (execution.ResolveCheckpointResult, error) {
	o.invocation = inv
	o.request = req
	o.denyCalled = true
	return o.denyResult, nil
}

type logOperationsRecorder struct {
	invocation  issue.Invocation
	addRequest  record.AddLogEntryRequest
	listRequest issue.LogListRequest
	addResult   record.AddLogEntryResult
	entries     []issue.LogEntry
}

func (o *logOperationsRecorder) AddLogEntry(_ context.Context, inv issue.Invocation, req record.AddLogEntryRequest) (record.AddLogEntryResult, error) {
	o.invocation = inv
	o.addRequest = req
	return o.addResult, nil
}

func (o *logOperationsRecorder) ListLogEntries(_ context.Context, req issue.LogListRequest) ([]issue.LogEntry, error) {
	o.listRequest = req
	return o.entries, nil
}

type stateOperationsRecorder struct {
	setRequest    record.SetStateRequest
	appendRequest record.SetStateRequest
	commitRequest record.CommitStateRequest
	readRequest   record.GetStateRequest
	stateResult   record.StateResult
	commitResult  record.CommitStateResult
	getResult     record.GetStateResult
}

func (o *stateOperationsRecorder) SetState(_ context.Context, _ issue.Invocation, req record.SetStateRequest) (record.StateResult, error) {
	o.setRequest = req
	return o.stateResult, nil
}

func (o *stateOperationsRecorder) AppendState(_ context.Context, _ issue.Invocation, req record.SetStateRequest) (record.StateResult, error) {
	o.appendRequest = req
	return o.stateResult, nil
}

func (o *stateOperationsRecorder) CommitState(_ context.Context, _ issue.Invocation, req record.CommitStateRequest) (record.CommitStateResult, error) {
	o.commitRequest = req
	return o.commitResult, nil
}

func (o *stateOperationsRecorder) GetState(_ context.Context, req record.GetStateRequest) (record.GetStateResult, error) {
	o.readRequest = req
	return o.getResult, nil
}

type resultOperationsRecorder struct {
	setRequest  record.SetResultRequest
	readRequest record.GetResultRequest
	setResult   record.SetResultResult
	issueResult issue.Result
}

func (o *resultOperationsRecorder) SetResult(_ context.Context, _ issue.Invocation, req record.SetResultRequest) (record.SetResultResult, error) {
	o.setRequest = req
	return o.setResult, nil
}

func (o *resultOperationsRecorder) GetResult(_ context.Context, req record.GetResultRequest) (issue.Result, error) {
	o.readRequest = req
	return o.issueResult, nil
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
