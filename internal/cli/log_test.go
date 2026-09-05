package cli

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/record"
	"go.uber.org/mock/gomock"
)

func TestLogShowID_UnmarshalText(t *testing.T) {
	tests := []struct {
		name    string
		give    string
		want    logShowID
		wantErr string
	}{
		{name: "Issue", give: "%cm-123", want: "cm-123"},
		{name: "CurrentLog", give: "log_11111111111111111111111111111111", want: "log_11111111111111111111111111111111"},
		{name: "HistoricalLog", give: "cmt_11111111111111111111111111111111", want: "cmt_11111111111111111111111111111111"},
		{name: "Invalid", give: "log_short", wantErr: "expected an issue ID or stable Log ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got logShowID
			err := got.UnmarshalText([]byte(test.give))
			if test.wantErr != "" {
				assert.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
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

	t.Run("ShowExactHistoricalEntry", func(t *testing.T) {
		request := record.GetLogEntryRequest{
			LogID: "cmt_22222222222222222222222222222222",
		}
		result := issue.LogEntry{
			ID: issue.LogID(request.LogID), IssueID: "an-1", Kind: "post",
			Author: new("a"), Committer: new("a"),
			Body: "first", Created: new(int64(10)),
		}
		operations := NewMockLogEntryReadOperations(gomock.NewController(t))
		operations.EXPECT().GetLogEntry(gomock.Any(), request).Return(result, nil)
		stdout, stderr, app := executionTestApplication(
			t,
			strings.NewReader(""),
			true,
			kong.BindTo(operations, (*LogEntryReadOperations)(nil)),
		)

		assert.Equal(t, ExitSuccess, app.Run(t.Context(), []string{
			"--json", "log", "show", request.LogID,
		}))
		assert.JSONEq(t, `{
			"id": "cmt_22222222222222222222222222222222",
			"issue_id": "an-1",
			"kind": "post",
			"author": "a",
			"committer": "a",
			"body": "first",
			"created": 10
		}`, stdout.String())
		assert.Empty(t, stderr.String())
		assert.Empty(t, (&logShowCommand{ID: logShowID(request.LogID)}).referencedIssueIDs())
	})
}
