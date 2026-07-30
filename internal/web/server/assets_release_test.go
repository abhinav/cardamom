//go:build assets

package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_servesConnectAndGeneratedAssetsOnOneListener(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	notice := newReadyWriter()
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Bind:        "127.0.0.1",
			Port:        0,
			NoBrowser:   true,
			Notice:      notice,
			HandlerPath: "/cardamom.private.v1.",
			Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "connect")
			}),
			AttachmentContentPath: "/attachments/",
			AttachmentContent: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, "attachment")
			}),
		})
	}()

	<-notice.ready
	address := strings.TrimSpace(strings.TrimPrefix(
		notice.String(),
		"web application available at ",
	))

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		address+"/cardamom.private.v1.ProjectService/GetBoard",
		nil,
	)
	require.NoError(t, err)
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "connect", string(body))

	request, err = http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		address+"/boards/current",
		nil,
	)
	require.NoError(t, err)
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	body, err = io.ReadAll(response.Body)
	closeErr = response.Body.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, string(body), `<div id="app"></div>`)

	request, err = http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		address+"/attachments/att_example/content",
		nil,
	)
	require.NoError(t, err)
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	body, err = io.ReadAll(response.Body)
	closeErr = response.Body.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "attachment", string(body))

	cancel()
	err = <-errCh
	assert.ErrorIs(t, err, context.Canceled)
}

// readyWriter makes the readiness notice a deterministic synchronization
// point without exposing a runtime-only test hook.
type readyWriter struct {
	ready chan struct{}
	once  sync.Once
	bytes.Buffer
}

func newReadyWriter() *readyWriter {
	return &readyWriter{ready: make(chan struct{})}
}

func (w *readyWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.once.Do(func() { close(w.ready) })
	return n, err
}
