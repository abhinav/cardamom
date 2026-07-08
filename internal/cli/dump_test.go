package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/dump"
)

func TestDumpCommandPassesSelectionForceAndRendersOneObject(t *testing.T) {
	var stdout, stderr bytes.Buffer
	selection := dump.NamedIssuesOnly("an-1", "an-2")
	operation := &recordingDumpOperation{result: dump.ExecutionResult{
		Destination: "published", BoardID: "board-1", Revision: 9,
		Selection: selection, Issues: 2, Written: 3, Unchanged: 1, Removed: 1,
	}}
	app, err := New(testConfig(&stdout, &stderr), kong.BindTo(operation, (*DumpOperation)(nil)))
	require.NoError(t, err)

	exitCode := app.Run(t.Context(), []string{
		"--json", "dump", "published", "--issue", "an-1", "--issue", "an-2",
		"--no-descendants", "--force",
	})

	assert.Equal(t, ExitSuccess, exitCode)
	destination, err := filepath.Abs("published")
	require.NoError(t, err)
	assert.Equal(t, dump.Request{
		Destination: destination, Selection: selection, Force: dump.ForceGenerated,
	}, operation.request)
	assert.JSONEq(t, `{
		"destination":"published","board_id":"board-1","revision":9,
		"selection":{"mode":"issues","issue_ids":["an-1","an-2"],
		"include_descendants":false},
		"issues":2,"written":3,"unchanged":1,"removed":1
	}`, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDumpCommandUsesWholeBoardAndHumanNotice(t *testing.T) {
	var stdout, stderr bytes.Buffer
	operation := &recordingDumpOperation{result: dump.ExecutionResult{
		Destination: "published", BoardID: "board-1", Revision: 9,
		Selection: dump.WholeBoard(), Issues: 4, Written: 5, Unchanged: 2,
	}}
	app, err := New(testConfig(&stdout, &stderr), kong.BindTo(operation, (*DumpOperation)(nil)))
	require.NoError(t, err)

	exitCode := app.Run(t.Context(), []string{"dump", "published"})

	assert.Equal(t, ExitSuccess, exitCode)
	destination, err := filepath.Abs("published")
	require.NoError(t, err)
	assert.Equal(t, dump.Request{
		Destination: destination, Selection: dump.WholeBoard(), Force: dump.PreserveGenerated,
	}, operation.request)
	assert.Equal(t, "published 4 issues to published (5 written, 2 unchanged, 0 removed)\n", stdout.String())
	assert.Empty(t, stderr.String())
}

type recordingDumpOperation struct {
	request dump.Request
	result  dump.ExecutionResult
}

func (o *recordingDumpOperation) Execute(
	_ context.Context,
	request dump.Request,
) (dump.ExecutionResult, error) {
	o.request = request
	return o.result, nil
}
