package cli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutput_humanStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	output := newOutput(&stdout, &stderr, false, false)

	require.NoError(t, output.WriteString("requested\n"))
	require.NoError(t, output.Noticef("created %s", "an-1"))
	require.NoError(t, output.Errorf("cannot open %s", "store"))

	assert.Equal(t, "requested\ncreated an-1\n", stdout.String())
	assert.Equal(t, "error: cannot open store\n", stderr.String())
}

func TestOutput_noticeSuppression(t *testing.T) {
	t.Run("Quiet", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		output := newOutput(&stdout, &stderr, false, true)

		require.NoError(t, output.Noticef("not printed"))
		require.NoError(t, output.Errorf("still printed"))

		assert.Empty(t, stdout.String())
		assert.Equal(t, "error: still printed\n", stderr.String())
	})

	t.Run("JSON", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		output := newOutput(&stdout, &stderr, true, false)

		require.NoError(t, output.Noticef("not printed"))
		require.NoError(t, output.Errorf("still printed"))

		assert.Empty(t, stdout.String())
		assert.Equal(t, "error: still printed\n", stderr.String())
	})
}

func TestOutput_WriteJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	output := newOutput(&stdout, &stderr, true, false)

	require.NoError(t, output.WriteJSON(struct {
		ID string `json:"id"`
	}{ID: "an-1"}))

	assert.Equal(t, "{\"id\":\"an-1\"}\n", stdout.String())
	assert.Empty(t, stderr.String())
}

func TestWriteJSONLines(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		output := newOutput(&stdout, &stderr, true, false)

		require.NoError(t, WriteJSONLines(output, []jsonRecord{}))

		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})

	t.Run("Records", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		output := newOutput(&stdout, &stderr, true, false)

		require.NoError(t, WriteJSONLines(output, []jsonRecord{
			{ID: "an-1"},
			{ID: "an-2"},
		}))

		assert.Equal(t, "{\"id\":\"an-1\"}\n{\"id\":\"an-2\"}\n", stdout.String())
		assert.Empty(t, stderr.String())
	})
}

func TestApplication_reportError(t *testing.T) {
	t.Run("Operation", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		app, err := New(testConfig(&stdout, &stderr))
		require.NoError(t, err)

		assert.Equal(t, ExitOperation, app.reportError(errors.New("store unavailable")))
		assert.Empty(t, stdout.String())
		assert.Equal(t, "error: store unavailable\n", stderr.String())
	})

	t.Run("Cancellation", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		app, err := New(testConfig(&stdout, &stderr))
		require.NoError(t, err)

		assert.Equal(t, ExitCanceled, app.reportError(contextCanceledError{}))
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	})
}

type jsonRecord struct {
	ID string `json:"id"`
}

type contextCanceledError struct{}

func (contextCanceledError) Error() string { return "wrapped cancellation" }

func (contextCanceledError) Unwrap() error { return context.Canceled }
