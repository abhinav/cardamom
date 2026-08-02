//go:build webdev

package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunDevelopment_forwardsViteArgumentsAndServesOneOrigin(t *testing.T) {
	webDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(webDir, "package.json"),
		[]byte(`{"scripts":{"dev":"node server.mjs"}}`),
		0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(webDir, "server.mjs"),
		[]byte(developmentServerFixture),
		0o600,
	))

	ctx, cancel := context.WithCancel(t.Context())
	notice := newDevelopmentReadyWriter()
	result := make(chan error, 1)
	go func() {
		result <- RunDevelopment(ctx, DevelopmentConfig{
			Config: Config{
				Bind: "127.0.0.1", Port: 0, NoBrowser: true, Notice: notice,
				HandlerPath: "/cardamom.private.v1.",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, "connect")
				}),
				AttachmentContentPattern: "/board/{boardID}/attachment/{attachmentID}",
				AttachmentContent: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = io.WriteString(w, "attachment")
				}),
			},
			WebDir: webDir,
		})
	}()

	select {
	case <-notice.ready:
	case err := <-result:
		require.NoError(t, err)
	}
	address := strings.TrimSpace(strings.TrimPrefix(
		notice.String(),
		"web application available at ",
	))
	assertResponse(t, address+"/", "development shell")
	assertResponse(t, address+"/cardamom.private.v1.ProjectService/GetBootstrap", "connect")
	assertResponse(t, address+"/board/board-1/attachment/att_example", "attachment")

	cancel()
	assert.ErrorIs(t, <-result, context.Canceled)
}

func assertResponse(t *testing.T, address, want string) {
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
	body, err := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	require.NoError(t, err)
	require.NoError(t, closeErr)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, want, string(body))
}

// developmentReadyWriter exposes the public readiness notice as a test
// synchronization point.
type developmentReadyWriter struct {
	ready chan struct{}
	once  sync.Once
	bytes.Buffer
}

func newDevelopmentReadyWriter() *developmentReadyWriter {
	return &developmentReadyWriter{ready: make(chan struct{})}
}

func (w *developmentReadyWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.once.Do(func() { close(w.ready) })
	return n, err
}

const developmentServerFixture = `
import { createServer } from "node:http";

const args = process.argv.slice(2);
if (args[0] === "--") {
  process.stderr.write("unexpected argument separator\n");
  process.exit(2);
}

const value = (name) => args[args.indexOf(name) + 1];
const server = createServer((_request, response) => {
  response.end("development shell");
});
server.listen(Number(value("--port")), value("--host"));
process.on("SIGTERM", () => server.close());
`
