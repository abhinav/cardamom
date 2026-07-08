package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewApplicationHandler_rejectsArchiveTraversal(t *testing.T) {
	_, err := newApplicationHandler(
		testArchive(t, map[string]string{"../outside.txt": "no"}),
		"/cardamom.private.v1.",
		http.NotFoundHandler(),
		"/attachments/",
		http.NotFoundHandler(),
	)

	require.Error(t, err)
	assert.ErrorContains(t, err, `invalid archive path "../outside.txt"`)
}

func TestNewApplicationHandler_servesConnectAndSPA(t *testing.T) {
	handler, err := newApplicationHandler(
		testArchive(t, map[string]string{
			"assets/application.js": "console.log('cardamom')",
			"index.html":            "<main>cardamom</main>",
		}),
		"/cardamom.private.v1.",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, "connect")
			require.NoError(t, err)
		}),
		"/attachments/",
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := io.WriteString(w, "attachment")
			require.NoError(t, err)
		}),
	)
	require.NoError(t, err)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "ProjectConnect", path: "/cardamom.private.v1.ProjectService/GetBoard", wantStatus: http.StatusOK, wantBody: "connect"},
		{name: "IssueConnect", path: "/cardamom.private.v1.IssueService/GetIssue", wantStatus: http.StatusOK, wantBody: "connect"},
		{name: "DumpConnect", path: "/cardamom.private.v1.DumpService/RenderDump", wantStatus: http.StatusOK, wantBody: "connect"},
		{name: "AttachmentContent", path: "/attachments/att_example/content", wantStatus: http.StatusOK, wantBody: "attachment"},
		{name: "Asset", path: "/assets/application.js", wantStatus: http.StatusOK, wantBody: "console.log('cardamom')"},
		{name: "BrowserRoute", path: "/issues/an-1", wantStatus: http.StatusOK, wantBody: "<main>cardamom</main>"},
		{name: "MissingAsset", path: "/assets/missing.js", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			assert.Equal(t, tt.wantStatus, response.Code)
			assert.Equal(t, tt.wantBody, response.Body.String())
		})
	}
}

func testArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, body := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		}))
		_, err := io.WriteString(tarWriter, body)
		require.NoError(t, err)
	}
	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())
	return compressed.Bytes()
}
