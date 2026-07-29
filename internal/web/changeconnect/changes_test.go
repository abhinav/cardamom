package changeconnect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"go.abhg.dev/cardamom/internal/board"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/project"
	"go.abhg.dev/cardamom/internal/web/boardscope"
)

func TestServiceWatchChangesStreamsCommittedInvalidations(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	subscription := newTestSubscription(CommittedChange{
		BoardID:  boardOne.ID(),
		Revision: 7,
	})
	source := &testSource{subscriptions: []*testSubscription{subscription}}
	client := newStreamingTestClient(t, testConfig{
		Catalog: &testCatalog{projects: []*project.State{projectOne}, boards: []*board.State{boardOne}},
		Changes: source,
	})

	stream, err := client.WatchChanges(
		t.Context(),
		connect.NewRequest(&privatev1.WatchChangesRequest{
			Scope: boardScope("board-1"),
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, stream.Close()) })
	require.True(t, stream.Receive())
	assert.Equal(t, "board-1", stream.Msg().GetBoardId())
	assert.Equal(t, uint64(7), stream.Msg().GetRevision())
	assert.Empty(t, stream.Msg().GetIssueId())
	assert.Equal(t, []privatev1.WatchResource{
		privatev1.WatchResource_WATCH_RESOURCE_BOARD_CATALOG,
		privatev1.WatchResource_WATCH_RESOURCE_BOARD,
		privatev1.WatchResource_WATCH_RESOURCE_ISSUES,
		privatev1.WatchResource_WATCH_RESOURCE_LOG,
		privatev1.WatchResource_WATCH_RESOURCE_APPROVALS,
	}, stream.Msg().GetResources())
	assert.Equal(t, []WatchRequest{{BoardIDs: []board.ID{"board-1"}}}, source.requestsSnapshot())
}

func TestServiceWatchChangesCancellationAndReconnect(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	first := newTestSubscription(CommittedChange{
		BoardID: boardOne.ID(), Revision: 10,
	})
	second := newTestSubscription(CommittedChange{
		BoardID: boardOne.ID(), Revision: 12,
	})
	source := &testSource{subscriptions: []*testSubscription{first, second}}
	client := newStreamingTestClient(t, testConfig{
		Catalog: &testCatalog{projects: []*project.State{projectOne}, boards: []*board.State{boardOne}},
		Changes: source,
	})

	firstContext, cancelFirst := context.WithCancel(t.Context())
	firstStream, err := client.WatchChanges(
		firstContext,
		connect.NewRequest(&privatev1.WatchChangesRequest{Scope: boardScope("board-1")}),
	)
	require.NoError(t, err)
	require.True(t, firstStream.Receive())
	assert.Equal(t, uint64(10), firstStream.Msg().GetRevision())
	first.waitForReceives(t, 2)
	cancelFirst()
	assert.False(t, firstStream.Receive())
	assert.Equal(t, connect.CodeCanceled, connect.CodeOf(firstStream.Err()))
	assert.NoError(t, firstStream.Close())

	secondStream, err := client.WatchChanges(
		t.Context(),
		connect.NewRequest(&privatev1.WatchChangesRequest{Scope: boardScope("board-1")}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, secondStream.Close()) })
	require.True(t, secondStream.Receive())
	assert.Equal(t, uint64(12), secondStream.Msg().GetRevision())
	assert.Len(t, source.requestsSnapshot(), 2)
}

func TestServiceWatchChangesPreservesAllBoardsScope(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	boardTwo := testBoard(t, "board-2", projectOne.ID(), "Board Two", nil)
	subscription := newTestSubscription(
		CommittedChange{
			BoardID: boardOne.ID(), Revision: 4,
		},
		CommittedChange{
			BoardID: boardTwo.ID(), Revision: 4,
		},
	)
	source := &testSource{subscriptions: []*testSubscription{subscription}}
	client := newStreamingTestClient(t, testConfig{
		Catalog: &testCatalog{
			projects: []*project.State{projectOne},
			boards:   []*board.State{boardOne, boardTwo},
		},
		Changes: source,
	})

	stream, err := client.WatchChanges(
		t.Context(),
		connect.NewRequest(&privatev1.WatchChangesRequest{
			Scope: &privatev1.BoardScope{
				Selection: &privatev1.BoardScope_AllBoards{AllBoards: &privatev1.AllBoards{}},
			},
		}),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, stream.Close()) })
	require.True(t, stream.Receive())
	assert.Equal(t, "board-1", stream.Msg().GetBoardId())
	require.True(t, stream.Receive())
	assert.Equal(t, "board-2", stream.Msg().GetBoardId())
	assert.Equal(t, uint64(4), stream.Msg().GetRevision())
	assert.Equal(t, []WatchRequest{{
		BoardIDs:  []board.ID{"board-1", "board-2"},
		AllBoards: true,
	}}, source.requestsSnapshot())
}

func TestServiceWatchChangesRejectsRepeatedBoardRevision(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)

	tests := []struct {
		name         string
		giveRevision uint64
	}{
		{name: "Duplicate", giveRevision: 5},
		{name: "Descending", giveRevision: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subscription := newTestSubscription(
				CommittedChange{
					BoardID: boardOne.ID(), Revision: 5,
				},
				CommittedChange{
					BoardID: boardOne.ID(), Revision: tt.giveRevision,
				},
			)
			client := newStreamingTestClient(t, testConfig{
				Catalog: &testCatalog{projects: []*project.State{projectOne}, boards: []*board.State{boardOne}},
				Changes: &testSource{subscriptions: []*testSubscription{subscription}},
			})

			stream, err := client.WatchChanges(
				t.Context(),
				connect.NewRequest(&privatev1.WatchChangesRequest{Scope: boardScope("board-1")}),
			)
			require.NoError(t, err)
			t.Cleanup(func() { assert.NoError(t, stream.Close()) })
			require.True(t, stream.Receive())
			assert.False(t, stream.Receive())
			assert.Equal(t, connect.CodeInternal, connect.CodeOf(stream.Err()))
		})
	}
}

func TestServiceWatchChangesRejectsInvalidScope(t *testing.T) {
	projectOne := testProject(t, "project-1", "Project One")
	boardOne := testBoard(t, "board-1", projectOne.ID(), "Board One", nil)
	source := &testSource{}
	client := newStreamingTestClient(t, testConfig{
		Catalog: &testCatalog{projects: []*project.State{projectOne}, boards: []*board.State{boardOne}},
		Changes: source,
	})

	stream, err := client.WatchChanges(
		t.Context(),
		connect.NewRequest(&privatev1.WatchChangesRequest{}),
	)
	if err == nil {
		t.Cleanup(func() { assert.NoError(t, stream.Close()) })
		assert.False(t, stream.Receive())
		err = stream.Err()
	}
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Empty(t, source.requestsSnapshot())
}

func boardScope(boardID string) *privatev1.BoardScope {
	return &privatev1.BoardScope{
		Selection: &privatev1.BoardScope_BoardId{BoardId: boardID},
	}
}

type testConfig struct {
	Catalog *testCatalog
	Changes Source
}

func newStreamingTestClient(t *testing.T, cfg testConfig) privatev1connect.ChangeServiceClient {
	t.Helper()
	service := New(Config{
		Scope:   boardscope.New(cfg.Catalog, &testIssueLocator{}),
		Changes: cfg.Changes,
	})
	_, handler := privatev1connect.NewChangeServiceHandler(service)
	client := &http.Client{Transport: &streamingTransport{handler: handler}}
	return privatev1connect.NewChangeServiceClient(client, "http://cardamom.test")
}

type streamingTransport struct {
	handler http.Handler
}

func (t *streamingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	reader, writer := io.Pipe()
	ready := make(chan responseHead, 1)
	response := &pipeResponseWriter{
		header: make(http.Header), body: writer, ready: ready,
	}
	go func() {
		t.handler.ServeHTTP(response, request)
		response.finish()
	}()
	head := <-ready
	return &http.Response{
		StatusCode: head.status,
		Status:     fmt.Sprintf("%d %s", head.status, http.StatusText(head.status)),
		Header:     head.header,
		Body:       reader,
		Request:    request,
	}, nil
}

type responseHead struct {
	status int
	header http.Header
}

type pipeResponseWriter struct {
	header http.Header
	body   *io.PipeWriter
	ready  chan<- responseHead
	once   sync.Once
}

func (w *pipeResponseWriter) Header() http.Header {
	return w.header
}

func (w *pipeResponseWriter) WriteHeader(status int) {
	w.once.Do(func() {
		w.ready <- responseHead{status: status, header: w.header.Clone()}
	})
}

func (w *pipeResponseWriter) Write(body []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(body)
}

func (w *pipeResponseWriter) Flush() {
	w.WriteHeader(http.StatusOK)
}

func (w *pipeResponseWriter) finish() {
	w.WriteHeader(http.StatusOK)
	_ = w.body.Close()
}

type testCatalog struct {
	projects []*project.State
	boards   []*board.State
}

func (c *testCatalog) Board(_ context.Context, id board.ID) (*board.State, error) {
	for _, board := range c.boards {
		if board.ID() == id {
			return board, nil
		}
	}
	return nil, errors.New("board not found")
}

func (c *testCatalog) ListAllBoards(context.Context) ([]*board.State, error) {
	return c.boards, nil
}

func (c *testCatalog) Get(ctx context.Context, id board.ID) (*board.State, error) {
	return c.Board(ctx, id)
}

func (c *testCatalog) List(ctx context.Context) ([]*board.State, error) {
	return c.ListAllBoards(ctx)
}

type testIssueLocator struct{}

func (*testIssueLocator) BoardForIssue(context.Context, string) (board.ID, error) {
	return "", errors.New("issue lookup is not used by change streams")
}

func testProject(t *testing.T, id, name string) *project.State {
	t.Helper()
	value, err := project.Load(project.Snapshot{
		ID: project.ID(id), Name: name, Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

func testBoard(
	t *testing.T,
	id string,
	projectID project.ID,
	name string,
	description *string,
) *board.State {
	t.Helper()
	value, err := board.Load(board.Snapshot{
		ID: board.ID(id), ProjectID: projectID.String(), Name: name,
		Description: description, Created: time.Unix(2, 0).UTC(),
	})
	require.NoError(t, err)
	return value
}

type testSource struct {
	mu            sync.Mutex
	subscriptions []*testSubscription
	requests      []WatchRequest
}

// Subscribe returns the next scripted subscription and records its request.
func (s *testSource) Subscribe(
	_ context.Context,
	request WatchRequest,
) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.subscriptions) == 0 {
		return nil, errors.New("test change subscription not configured")
	}
	subscription := s.subscriptions[0]
	s.subscriptions = s.subscriptions[1:]
	return subscription, nil
}

func (s *testSource) requestsSnapshot() []WatchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]WatchRequest(nil), s.requests...)
}

type testChangeResult struct {
	change CommittedChange
	err    error
}

type testSubscription struct {
	results  chan testChangeResult
	receives chan struct{}
}

func newTestSubscription(changes ...CommittedChange) *testSubscription {
	subscription := &testSubscription{
		results:  make(chan testChangeResult, len(changes)),
		receives: make(chan struct{}, len(changes)+1),
	}
	for _, change := range changes {
		subscription.results <- testChangeResult{change: change}
	}
	return subscription
}

// Receive returns the next scripted change after exposing a synchronization
// point to the test.
func (s *testSubscription) Receive(ctx context.Context) (CommittedChange, error) {
	s.receives <- struct{}{}
	select {
	case <-ctx.Done():
		return CommittedChange{}, ctx.Err()
	case result := <-s.results:
		return result.change, result.err
	}
}

func (s *testSubscription) waitForReceives(t *testing.T, count int) {
	t.Helper()
	for range count {
		<-s.receives
	}
}
