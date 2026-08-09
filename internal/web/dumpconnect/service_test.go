package dumpconnect

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/dump"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.uber.org/mock/gomock"
)

func TestServiceRenderDumpStreamsManifestThenCanonicalFiles(t *testing.T) {
	firstBody := bytes.Repeat([]byte("a"), 64*1024+1)
	first, firstReader := testGeneratedFile(t, "issues/an-a.md", "issue:an-a", firstBody)
	second, secondReader := testGeneratedFile(t, "issues/an-c.md", "issue:an-c", []byte("second\n"))
	result := dump.RenderedDump{
		Provenance: dump.Provenance{
			ProjectID: "project-1", ProjectName: "Project one",
			BoardID: "board-1", BoardName: "Board one",
		},
		Revision:   12,
		Selection:  dump.NamedIssuesOnly("an-a", "an-c"),
		IssueCount: 2,
		Files:      []*dump.GeneratedFile{first, second},
	}
	renderer := NewMockRenderer(gomock.NewController(t))
	renderer.EXPECT().Render(gomock.Any(), dump.RenderRequest{
		Selection: dump.NamedIssuesOnly("an-c", "an-a", "an-c"),
	}).Return(result, nil)
	factory := NewMockRendererFactory(gomock.NewController(t))
	factory.EXPECT().Renderer(
		gomock.Any(), board.ID("board-1"),
	).Return(renderer, nil)
	client := newTestClient(t, New(Config{Renderers: factory}))

	stream, err := client.RenderDump(t.Context(), connect.NewRequest(&privatev1.RenderDumpRequest{
		BoardId: "board-1",
		Selection: &privatev1.DumpSelection{Mode: &privatev1.DumpSelection_Issues{
			Issues: &privatev1.IssueDumpSelection{
				IssueIds: []string{"an-c", "an-a", "an-c"},
			},
		}},
	}))
	require.NoError(t, err)

	var frames []*privatev1.RenderDumpResponse
	for stream.Receive() {
		frames = append(frames, stream.Msg())
	}
	require.NoError(t, stream.Err())
	require.Len(t, frames, 4)

	manifest := frames[0].GetManifest()
	require.NotNil(t, manifest)
	assert.Equal(t, "project-1", manifest.GetProjectId())
	assert.Equal(t, "Project one", manifest.GetProjectName())
	assert.Equal(t, "board-1", manifest.GetBoardId())
	assert.Equal(t, "Board one", manifest.GetBoardName())
	assert.Equal(t, uint64(12), manifest.GetRevision())
	assert.Equal(t, uint32(2), manifest.GetIssueCount())
	assert.Equal(t, []string{"an-a", "an-c"}, manifest.GetSelection().GetIssues().GetIssueIds())
	assert.False(t, manifest.GetSelection().GetIssues().GetIncludeDescendants())
	assert.Equal(t, []*privatev1.DumpFileManifest{
		{Path: "issues/an-a.md", Identity: "issue:an-a", SizeBytes: uint64(len(firstBody))},
		{Path: "issues/an-c.md", Identity: "issue:an-c", SizeBytes: 7},
	}, manifest.GetFiles())
	assert.Equal(t, &privatev1.DumpFileChunk{
		FileIndex: 0, Offset: 0, Content: firstBody[:64*1024],
	}, frames[1].GetFileChunk())
	assert.Equal(t, &privatev1.DumpFileChunk{
		FileIndex: 0, Offset: 64 * 1024, Content: firstBody[64*1024:],
	}, frames[2].GetFileChunk())
	assert.Equal(t, &privatev1.DumpFileChunk{
		FileIndex: 1, Offset: 0, Content: []byte("second\n"),
	}, frames[3].GetFileChunk())
	assert.True(t, firstReader.closed)
	assert.True(t, secondReader.closed)
	assert.Greater(t, firstReader.readCalls, 1)
}

func TestServiceRenderDumpRejectsMissingSelectionBeforeOpeningRenderer(t *testing.T) {
	factory := NewMockRendererFactory(gomock.NewController(t))
	client := newTestClient(t, New(Config{Renderers: factory}))

	stream, err := client.RenderDump(t.Context(), connect.NewRequest(&privatev1.RenderDumpRequest{
		BoardId: "board-1",
	}))
	require.NoError(t, err)
	assert.False(t, stream.Receive())
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(stream.Err()))
}

func TestManifestFromRenderedRejectsNilGeneratedFile(t *testing.T) {
	_, err := manifestFromRendered(dump.RenderedDump{
		Selection: dump.WholeBoard(),
		Files:     []*dump.GeneratedFile{nil},
	})

	assert.EqualError(t, err, "dump generated file is required")
}

type recordingReadCloser struct {
	*bytes.Reader
	maxRead   int
	readCalls int
	closed    bool
}

func (r *recordingReadCloser) Read(body []byte) (int, error) {
	if len(body) > r.maxRead {
		return 0, fmt.Errorf("read buffer is %d bytes, maximum is %d", len(body), r.maxRead)
	}
	r.readCalls++
	return r.Reader.Read(body)
}

func (r *recordingReadCloser) Close() error {
	r.closed = true
	return nil
}

func testGeneratedFile(
	t *testing.T,
	path string,
	identity string,
	body []byte,
) (*dump.GeneratedFile, *recordingReadCloser) {
	t.Helper()
	reader := &recordingReadCloser{
		Reader:  bytes.NewReader(body),
		maxRead: 64 * 1024,
	}
	file, err := dump.NewGeneratedFile(dump.GeneratedFileConfig{
		Path: path, Identity: identity, Size: int64(len(body)),
		Open: func() (io.ReadCloser, error) { return reader, nil },
	})
	require.NoError(t, err)
	return file, reader
}

func newTestClient(t *testing.T, service *Service) privatev1connect.DumpServiceClient {
	t.Helper()
	path, handler := privatev1connect.NewDumpServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder.Result(), nil
	})}
	return privatev1connect.NewDumpServiceClient(client, "http://cardamom.test")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
