package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSharedLifetime_browserLaunchFailureKeepsServing(t *testing.T) {
	diagnosticReader, diagnosticWriter := io.Pipe()
	t.Cleanup(func() {
		assert.NoError(t, diagnosticReader.Close())
		assert.NoError(t, diagnosticWriter.Close())
	})

	address, cancel, result := startBrowserFailureServer(t, diagnosticWriter)
	warning, err := bufio.NewReader(diagnosticReader).ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, warning, "warning: open browser at")

	assertServerAvailable(t, address)

	cancel()
	assert.ErrorIs(t, <-result, context.Canceled)
}

func TestRunSharedLifetime_browserDiagnosticFailureKeepsServing(t *testing.T) {
	diagnostic := &failingDiagnosticWriter{
		attempted: make(chan string),
		completed: make(chan struct{}),
	}
	_, cancel, result := startBrowserFailureServer(t, diagnostic)

	warning := <-diagnostic.attempted
	assert.Contains(t, warning, "warning: open browser at")
	<-diagnostic.completed

	cancel()
	assert.ErrorIs(t, <-result, context.Canceled)
}

func startBrowserFailureServer(
	t *testing.T,
	diagnostic io.Writer,
) (address string, cancel context.CancelFunc, result <-chan error) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	ctx, cancel := context.WithCancel(t.Context())
	noticeReader, noticeWriter := io.Pipe()
	t.Cleanup(func() {
		assert.NoError(t, noticeReader.Close())
		assert.NoError(t, noticeWriter.Close())
	})

	resultChannel := make(chan error, 1)
	go func() {
		resultChannel <- runSharedLifetime(
			ctx,
			Config{
				Bind: "127.0.0.1", Port: 0,
				Notice: noticeWriter, Diagnostic: diagnostic,
				HandlerPath: "/cardamom.private.v1.", Handler: http.NotFoundHandler(),
				AttachmentContentPattern: "/board/{boardID}/attachment/{attachmentID}",
				AttachmentContent:        http.NotFoundHandler(),
			},
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "cardamom")
			}),
			nil,
		)
	}()

	readiness, err := bufio.NewReader(noticeReader).ReadString('\n')
	require.NoError(t, err)
	address = strings.TrimSpace(strings.TrimPrefix(
		readiness,
		"web application available at ",
	))
	return address, cancel, resultChannel
}

func assertServerAvailable(t *testing.T, address string) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		address,
		nil,
	)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.NoError(t, response.Body.Close())
}

// failingDiagnosticWriter records the attempted warning before rejecting it.
type failingDiagnosticWriter struct {
	// attempted receives the warning passed to Write.
	attempted chan string

	// completed closes as Write returns its failure.
	completed chan struct{}
}

func (w *failingDiagnosticWriter) Write(p []byte) (int, error) {
	defer close(w.completed)
	w.attempted <- string(p)
	return 0, errors.New("diagnostic unavailable")
}
