package aggregate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
	"go.abhg.dev/cardamom/internal/web"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewBuildsBootstrapAndReadOnlyHandler(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-primary", ProjectId: "project-primary", Name: "Primary"}
	server := newSourceServer(t, sourceBootstrap(board, true), &v1.Board{
		Id: board.GetId(), ProjectId: board.GetProjectId(), Name: board.GetName(),
	})

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "primary", URL: mustURL(t, server.URL)}},
		Version: "aggregate-test",
	})
	require.NoError(t, err)

	client := newAggregateClient(t, aggregate)
	bootstrap, err := client.GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_HEALTHY, bootstrap.Msg.GetSources()[0].GetHealth())
	assert.Equal(t, "primary", bootstrap.Msg.GetSources()[0].GetSource().GetSourceId())
	assert.Equal(t, "lineage-primary", bootstrap.Msg.GetSources()[0].GetSource().GetStoreLineageId())
	assert.Equal(t, v1.AccessMode_ACCESS_MODE_READ_ONLY, bootstrap.Msg.GetAccessMode())
	assert.Equal(t, "board-primary", bootstrap.Msg.GetBoards()[0].GetId())
	assert.Equal(t, "primary", bootstrap.Msg.GetBoards()[0].GetSource().GetSourceId())
	_, ok := bootstrap.Msg.GetDefaultScope().GetSelection().(*v1.BoardScope_AllSources)
	assert.True(t, ok)

	_, err = client.CreateBoard(t.Context(), connect.NewRequest(&v1.CreateBoardRequest{}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestNewStartsEmptyAggregate(t *testing.T) {
	aggregate, err := New(t.Context(), Config{Version: "empty-aggregate"})
	require.NoError(t, err)

	bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	assert.Empty(t, bootstrap.Msg.GetSources())
	assert.Empty(t, bootstrap.Msg.GetProjects())
	assert.Empty(t, bootstrap.Msg.GetBoards())
	assert.Equal(t, v1.AccessMode_ACCESS_MODE_READ_ONLY, bootstrap.Msg.GetAccessMode())
	assert.True(t, bootstrap.Msg.GetAggregateStatus().GetComplete())
	_, ok := bootstrap.Msg.GetDefaultScope().GetSelection().(*v1.BoardScope_AllSources)
	assert.True(t, ok)

	issues, err := newAggregateIssueClient(t, aggregate).ListIssues(
		t.Context(), connect.NewRequest(&v1.ListIssuesRequest{
			Scope: &v1.BoardScope{Selection: &v1.BoardScope_AllSources{
				AllSources: &v1.AllSources{},
			}},
		}),
	)
	require.NoError(t, err)
	assert.Empty(t, issues.Msg.GetIssues())
	assert.True(t, issues.Msg.GetAggregateStatus().GetComplete())
}

func TestNewAcceptsWritableSource(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-writable", ProjectId: "project", Name: "Writable"}
	server := newSourceServer(t, sourceBootstrap(board, false), nil)

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "source", URL: mustURL(t, server.URL)}},
	})
	require.NoError(t, err)

	bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_HEALTHY, bootstrap.Msg.GetSources()[0].GetHealth())
	assert.Len(t, bootstrap.Msg.GetBoards(), 1)
}

func TestNewRejectsInvalidSourceAliases(t *testing.T) {
	for _, alias := range []string{"", "1source", "source/name"} {
		t.Run(alias, func(t *testing.T) {
			_, err := New(t.Context(), Config{
				Sources: []SourceConfig{{Alias: alias, URL: mustURL(t, "http://source.test")}},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "source alias")
		})
	}
}

func TestNewAcceptsHyphenatedSourceAlias(t *testing.T) {
	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{
			Alias: "source-name",
			URL:   mustURL(t, "http://source.test"),
		}},
	})
	require.NoError(t, err)

	bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, "source-name", bootstrap.Msg.GetSources()[0].GetSource().GetSourceId())
}

func TestAggregateRoutesStateAndAttachmentsToSource(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-records", ProjectId: "project", Name: "Records"}
	state := &v1.StateRecord{Actor: new("worker")}
	attachment := &v1.Attachment{Id: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa", BoardId: board.GetId()}
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
		issue: func(_ context.Context, request *connect.Request[v1.GetIssueRequest]) (*connect.Response[v1.GetIssueResponse], error) {
			return connect.NewResponse(&v1.GetIssueResponse{Issue: &v1.IssueDetail{
				Issue: &v1.IssueSummary{Id: request.Msg.GetIssueId(), BoardId: board.GetId()},
			}}), nil
		},
		state: func(_ context.Context, request *connect.Request[v1.GetStateRequest]) (*connect.Response[v1.GetStateResponse], error) {
			assert.Equal(t, "issue-records", request.Msg.GetIssueId())
			assert.Nil(t, request.Msg.GetSource())
			assert.Empty(t, request.Msg.GetBoardId())
			return connect.NewResponse(&v1.GetStateResponse{IssueId: "issue-records", State: state}), nil
		},
		attachments: func(_ context.Context, request *connect.Request[v1.ListAttachmentsRequest]) (*connect.Response[v1.ListAttachmentsResponse], error) {
			assert.Equal(t, board.GetId(), request.Msg.GetBoardId())
			assert.Equal(t, "issue-records", request.Msg.GetIssueId())
			assert.Nil(t, request.Msg.GetSource())
			return connect.NewResponse(&v1.ListAttachmentsResponse{Attachments: []*v1.Attachment{attachment}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "primary", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)

	source := &v1.SourceRef{SourceId: "primary", StoreLineageId: "lineage-primary"}
	stateResponse, err := newAggregateRecordClient(t, aggregate).GetState(t.Context(), connect.NewRequest(&v1.GetStateRequest{
		IssueId: "issue-records", Source: source, BoardId: new(board.GetId()),
	}))
	require.NoError(t, err)
	assert.Equal(t, state.GetActor(), stateResponse.Msg.GetState().GetActor())

	attachments, err := newAggregateAttachmentClient(t, aggregate).ListAttachments(t.Context(), connect.NewRequest(&v1.ListAttachmentsRequest{
		BoardId: board.GetId(), IssueId: new("issue-records"), Source: source,
	}))
	require.NoError(t, err)
	require.Len(t, attachments.Msg.GetAttachments(), 1)
	assert.Equal(t, "primary", attachments.Msg.GetAttachments()[0].GetSource().GetSourceId())
}

func TestAggregateRejectsStateForAnotherBoard(t *testing.T) {
	first := &v1.BoardSummary{Id: "board-first", ProjectId: "project", Name: "First"}
	second := &v1.BoardSummary{Id: "board-second", ProjectId: "project", Name: "Second"}
	bootstrap := sourceBootstrap(first, true)
	bootstrap.Boards = append(bootstrap.Boards, second)
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrap,
		issue: func(_ context.Context, request *connect.Request[v1.GetIssueRequest]) (*connect.Response[v1.GetIssueResponse], error) {
			return connect.NewResponse(&v1.GetIssueResponse{Issue: &v1.IssueDetail{
				Issue: &v1.IssueSummary{Id: request.Msg.GetIssueId(), BoardId: first.GetId()},
			}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "primary", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)

	_, err = newAggregateRecordClient(t, aggregate).GetState(t.Context(), connect.NewRequest(&v1.GetStateRequest{
		IssueId: "issue-first",
		Source:  &v1.SourceRef{SourceId: "primary", StoreLineageId: "lineage-primary"},
		BoardId: new(second.GetId()),
	}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestNewAllowsDuplicateBoardIDsAcrossStoreLineages(t *testing.T) {
	boardID := "board-copied"
	originalBoard := &v1.BoardSummary{
		Id: boardID, ProjectId: "project-original", Name: "Original",
		Archived: &v1.BoardArchive{Actor: "operator"},
	}
	restoredBoard := &v1.BoardSummary{
		Id: boardID, ProjectId: "project-restored", Name: "Restored",
	}
	first := newSourceServer(t, sourceBootstrap(originalBoard, true), &v1.Board{
		Id: boardID, ProjectId: originalBoard.GetProjectId(), Name: originalBoard.GetName(),
		Archived: originalBoard.GetArchived(),
	})
	secondBootstrap := sourceBootstrap(restoredBoard, true)
	secondBootstrap.Sources[0].Source.StoreLineageId = "lineage-second"
	second := newSourceServer(t, secondBootstrap, &v1.Board{
		Id: boardID, ProjectId: restoredBoard.GetProjectId(), Name: restoredBoard.GetName(),
	})

	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "first", URL: mustURL(t, first.URL)},
		{Alias: "second", URL: mustURL(t, second.URL)},
	}})
	require.NoError(t, err)

	client := newAggregateClient(t, aggregate)
	bootstrap, err := client.GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetBoards(), 2)
	assert.NotNil(t, bootstrap.Msg.GetBoards()[0].GetArchived())
	assert.Nil(t, bootstrap.Msg.GetBoards()[1].GetArchived())

	_, err = client.GetBoard(
		t.Context(), connect.NewRequest(&v1.GetBoardRequest{BoardId: boardID}),
	)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	response, err := client.GetBoard(t.Context(), connect.NewRequest(&v1.GetBoardRequest{
		BoardId: boardID,
		Source: &v1.SourceRef{
			SourceId: "second", StoreLineageId: "lineage-second",
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, "Restored", response.Msg.GetBoard().GetName())
	assert.Equal(t, "second", response.Msg.GetBoard().GetSource().GetSourceId())
}

func TestNewRejectsDuplicateStoreLineages(t *testing.T) {
	first := newSourceServer(t, sourceBootstrap(&v1.BoardSummary{
		Id: "board-first", ProjectId: "project-first", Name: "First",
	}, true), nil)
	second := newSourceServer(t, sourceBootstrap(&v1.BoardSummary{
		Id: "board-second", ProjectId: "project-second", Name: "Second",
	}, true), nil)

	_, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "first", URL: mustURL(t, first.URL)},
		{Alias: "second", URL: mustURL(t, second.URL)},
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `store lineage "lineage-primary" is configured under sources "first" and "second"`)
}

func TestNewKeepsUnavailableSourceInBootstrap(t *testing.T) {
	server := newSourceServer(t, sourceBootstrap(nil, true), nil)
	server.Close()

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "offline", URL: mustURL(t, server.URL)}},
	})
	require.NoError(t, err)

	bootstrap, err := newAggregateClient(t, aggregate).GetBootstrap(
		t.Context(), connect.NewRequest(&v1.GetBootstrapRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, bootstrap.Msg.GetSources(), 1)
	assert.Equal(t, "offline", bootstrap.Msg.GetSources()[0].GetSource().GetSourceId())
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_UNAVAILABLE, bootstrap.Msg.GetSources()[0].GetHealth())
	assert.False(t, bootstrap.Msg.GetAggregateStatus().GetComplete())
	assert.Equal(t, "source unavailable", bootstrap.Msg.GetAggregateStatus().GetProblems()[0].GetSummary())
}

func TestProjectServiceGetBoardUsesCanonicalBoardRoute(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-canonical", ProjectId: "project", Name: "Canonical"}
	source := newSourceServer(t, sourceBootstrap(board, true), &v1.Board{
		Id: board.GetId(), ProjectId: board.GetProjectId(), Name: board.GetName(),
	})

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "laptop", URL: mustURL(t, source.URL)}},
	})
	require.NoError(t, err)
	response, err := newAggregateClient(t, aggregate).GetBoard(
		t.Context(), connect.NewRequest(&v1.GetBoardRequest{BoardId: board.GetId()}),
	)
	require.NoError(t, err)
	assert.Equal(t, board.GetId(), response.Msg.GetBoard().GetId())
	assert.Equal(t, "laptop", response.Msg.GetBoard().GetSource().GetSourceId())
}

func TestAttachmentContentProxyPreservesRangeAndConditionalHeaders(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-attach", ProjectId: "project", Name: "Attachments"}
	attachmentID := "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	var gotRange, gotIfNoneMatch string
	source := newConfiguredSourceServerWithRoutes(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/board/board-attach/attachment/"+attachmentID, r.URL.Path)
		gotRange = r.Header.Get("Range")
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("ETag", `"source-v1"`)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Set-Cookie", "source=secret")
		if gotIfNoneMatch == `"source-v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		assert.Equal(t, "bytes=2-5", gotRange)
		w.Header().Set("Content-Range", "bytes 2-5/10")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "2345")
	}))
	mountedSource := httptest.NewServer(http.StripPrefix(
		"/source",
		httputil.NewSingleHostReverseProxy(mustURL(t, source.URL)),
	))
	t.Cleanup(mountedSource.Close)

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "media", URL: mustURL(t, mountedSource.URL+"/source")}},
	})
	require.NoError(t, err)
	binding := aggregate.Binding()
	mux := http.NewServeMux()
	mux.Handle(binding.AttachmentContentPattern, binding.AttachmentContent)

	request := httptest.NewRequest(
		http.MethodGet,
		"/source/media/board/board-attach/attachment/"+attachmentID,
		nil,
	)
	request.Header.Set("Range", "bytes=2-5")
	request.Header.Set("Authorization", "Bearer browser-secret")
	request.Header.Set("Cookie", "session=browser-secret")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	assert.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, "2345", response.Body.String())
	assert.Equal(t, "bytes=2-5", gotRange)
	assert.Equal(t, `"source-v1"`, response.Header().Get("ETag"))
	assert.Equal(t, "bytes 2-5/10", response.Header().Get("Content-Range"))
	assert.Empty(t, response.Header().Get("Set-Cookie"))

	request = httptest.NewRequest(
		http.MethodGet,
		"/source/media/board/board-attach/attachment/"+attachmentID,
		nil,
	)
	request.Header.Set("If-None-Match", `"source-v1"`)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotModified, response.Code)
	assert.Empty(t, response.Body.String())
	assert.Equal(t, `"source-v1"`, gotIfNoneMatch)
}

func TestAttachmentContentProxyUsesSourceQualifiedBoardRoute(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-copied", ProjectId: "project", Name: "Copied"}
	firstBootstrap := sourceBootstrap(board, true)
	firstBootstrap.Sources[0].Source.StoreLineageId = "lineage-first"
	first := newConfiguredSourceServerWithRoutes(t, &sourceHandler{
		bootstrap: firstBootstrap,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "first")
	}))
	secondBootstrap := sourceBootstrap(board, true)
	secondBootstrap.Sources[0].Source.StoreLineageId = "lineage-second"
	second := newConfiguredSourceServerWithRoutes(t, &sourceHandler{
		bootstrap: secondBootstrap,
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "second")
	}))
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "first", URL: mustURL(t, first.URL)},
		{Alias: "second", URL: mustURL(t, second.URL)},
	}})
	require.NoError(t, err)
	binding := aggregate.Binding()
	mux := http.NewServeMux()
	mux.Handle(binding.AttachmentContentPattern, binding.AttachmentContent)

	request := httptest.NewRequest(
		http.MethodGet,
		"/source/second/board/board-copied/attachment/att_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		nil,
	)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "second", response.Body.String())
}

func TestAttachmentContentProxyPropagatesCancellation(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-cancel", ProjectId: "project", Name: "Cancellation"}
	attachmentID := "att_aaaaaaaaaaaaaaaaaaaaaaaaaa"
	started := make(chan struct{})
	sourceCanceled := make(chan struct{})
	source := newConfiguredSourceServerWithRoutes(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
	}, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(sourceCanceled)
	}))

	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "cancel", URL: mustURL(t, source.URL)}},
	})
	require.NoError(t, err)
	binding := aggregate.Binding()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := httptest.NewRequest(
		http.MethodGet,
		"/source/cancel/board/board-cancel/attachment/"+attachmentID,
		nil,
	)
	request = request.WithContext(ctx)
	request.SetPathValue("sourceID", "cancel")
	request.SetPathValue("boardID", board.GetId())
	request.SetPathValue("attachmentID", attachmentID)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		binding.AttachmentContent.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("source request did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("aggregate request did not finish after cancellation")
	}
	select {
	case <-sourceCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("source request was not canceled")
	}
}

func TestIssueReadsMergePagesWithPartialSourceHealth(t *testing.T) {
	boardA := &v1.BoardSummary{Id: "board-a", ProjectId: "project-a", Name: "A"}
	boardB := &v1.BoardSummary{Id: "board-b", ProjectId: "project-b", Name: "B"}
	bootstrapA := sourceBootstrap(boardA, true)
	bootstrapA.Sources[0].Source.StoreLineageId = "lineage-a"
	bootstrapB := sourceBootstrap(boardB, true)
	bootstrapB.Sources[0].Source.StoreLineageId = "lineage-b"

	serverA := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrapA,
		issues: func(_ context.Context, request *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			if request.Msg.GetPageToken() == "a2" {
				return connect.NewResponse(&v1.ListIssuesResponse{
					Issues: []*v1.IssueSummary{{Id: "issue-e", BoardId: "board-a", Title: "Echo", Priority: 1}},
				}), nil
			}
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues: []*v1.IssueSummary{
					{Id: "issue-a", BoardId: "board-a", Title: "Alpha", Priority: 1},
					{Id: "issue-c", BoardId: "board-a", Title: "Charlie", Priority: 1},
				},
				Truncated: true, NextPageToken: new("a2"), TotalCount: 3,
			}), nil
		},
	})
	serverB := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrapB,
		issues: func(_ context.Context, _ *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues: []*v1.IssueSummary{
					{Id: "issue-b", BoardId: "board-b", Title: "Bravo", Priority: 1},
					{Id: "issue-d", BoardId: "board-b", Title: "Delta", Priority: 1},
				},
				TotalCount: 2,
			}), nil
		},
	})
	offline := newSourceServer(t, sourceBootstrap(nil, true), nil)
	offline.Close()

	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "alpha", URL: mustURL(t, serverA.URL)},
		{Alias: "bravo", URL: mustURL(t, serverB.URL)},
		{Alias: "offline", URL: mustURL(t, offline.URL)},
	}})
	require.NoError(t, err)
	client := newAggregateIssueClient(t, aggregate)
	request := &v1.ListIssuesRequest{
		Scope: &v1.BoardScope{Selection: &v1.BoardScope_AllSources{AllSources: &v1.AllSources{}}},
		Sort:  v1.IssueSort_ISSUE_SORT_TITLE,
		Limit: 3,
	}
	first, err := client.ListIssues(t.Context(), connect.NewRequest(request))
	require.NoError(t, err)
	assert.Equal(t, []string{"Alpha", "Bravo", "Charlie"}, issueTitles(first.Msg.GetIssues()))
	assert.Equal(t, uint32(5), first.Msg.GetTotalCount())
	assert.False(t, first.Msg.GetAggregateStatus().GetComplete())
	assert.Equal(t, "offline", first.Msg.GetAggregateStatus().GetProblems()[0].GetSourceId())
	assert.Equal(t, "alpha", first.Msg.GetIssues()[0].GetSource().GetSourceId())
	require.NotNil(t, first.Msg.GetNextPageToken())

	second, err := client.ListIssues(t.Context(), connect.NewRequest(&v1.ListIssuesRequest{
		Scope:     request.Scope,
		Sort:      request.Sort,
		Limit:     request.Limit,
		PageToken: first.Msg.NextPageToken,
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"Delta", "Echo"}, issueTitles(second.Msg.GetIssues()))
	assert.False(t, second.Msg.GetTruncated())
}

func TestIssueReadsDefaultToCreationOrderAcrossSources(t *testing.T) {
	boardA := &v1.BoardSummary{Id: "board-a", ProjectId: "project-a", Name: "A"}
	boardB := &v1.BoardSummary{Id: "board-b", ProjectId: "project-b", Name: "B"}
	bootstrapA := sourceBootstrap(boardA, true)
	bootstrapA.Sources[0].Source.StoreLineageId = "lineage-a"
	bootstrapB := sourceBootstrap(boardB, true)
	bootstrapB.Sources[0].Source.StoreLineageId = "lineage-b"
	serverA := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrapA,
		issues: func(_ context.Context, _ *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues: []*v1.IssueSummary{{
					Id: "issue-z", BoardId: boardA.GetId(), Title: "Older",
					Priority: 4, CreatedAt: timestamppb.New(time.Unix(1, 0)),
				}},
				TotalCount: 1,
			}), nil
		},
	})
	serverB := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrapB,
		issues: func(_ context.Context, _ *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues: []*v1.IssueSummary{{
					Id: "issue-a", BoardId: boardB.GetId(), Title: "Newer",
					Priority: 0, CreatedAt: timestamppb.New(time.Unix(2, 0)),
				}},
				TotalCount: 1,
			}), nil
		},
	})

	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "alpha", URL: mustURL(t, serverA.URL)},
		{Alias: "bravo", URL: mustURL(t, serverB.URL)},
	}})
	require.NoError(t, err)
	response, err := newAggregateIssueClient(t, aggregate).ListIssues(
		t.Context(),
		connect.NewRequest(&v1.ListIssuesRequest{
			Scope: &v1.BoardScope{Selection: &v1.BoardScope_AllSources{
				AllSources: &v1.AllSources{},
			}},
		}),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"Older", "Newer"}, issueTitles(response.Msg.GetIssues()))
}

func TestIssueReadsRespectSourceAndProjectScopes(t *testing.T) {
	alphaBoardA := &v1.BoardSummary{Id: "alpha-a", ProjectId: "project-a", Name: "Alpha A"}
	alphaBoardB := &v1.BoardSummary{Id: "alpha-b", ProjectId: "project-b", Name: "Alpha B"}
	betaBoardB := &v1.BoardSummary{Id: "beta-b", ProjectId: "project-b", Name: "Beta B"}

	alphaBootstrap := sourceBootstrap(nil, true)
	alphaBootstrap.Sources[0].Source.StoreLineageId = "lineage-alpha"
	alphaBootstrap.Projects = []*v1.Project{
		{Id: "project-a", Name: "Project A"},
		{Id: "project-b", Name: "Project B"},
	}
	alphaBootstrap.Boards = []*v1.BoardSummary{alphaBoardA, alphaBoardB}
	betaBootstrap := sourceBootstrap(betaBoardB, true)
	betaBootstrap.Sources[0].Source.StoreLineageId = "lineage-beta"

	var alphaScopes []*v1.BoardScope
	alpha := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: alphaBootstrap,
		issues: func(_ context.Context, request *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			alphaScopes = append(alphaScopes, request.Msg.GetScope())
			if request.Msg.GetScope().GetBoardId() == alphaBoardB.GetId() {
				return connect.NewResponse(&v1.ListIssuesResponse{
					Issues:     []*v1.IssueSummary{{Id: "alpha-issue-b", BoardId: alphaBoardB.GetId(), Title: "Alpha B"}},
					TotalCount: 1,
				}), nil
			}
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues: []*v1.IssueSummary{
					{Id: "alpha-issue-a", BoardId: alphaBoardA.GetId(), Title: "Alpha A"},
					{Id: "alpha-issue-b", BoardId: alphaBoardB.GetId(), Title: "Alpha B"},
				},
				TotalCount: 2,
			}), nil
		},
	})
	betaCalls := 0
	beta := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: betaBootstrap,
		issues: func(_ context.Context, _ *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			betaCalls++
			return connect.NewResponse(&v1.ListIssuesResponse{
				Issues:     []*v1.IssueSummary{{Id: "beta-issue-b", BoardId: betaBoardB.GetId(), Title: "Beta B"}},
				TotalCount: 1,
			}), nil
		},
	})

	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "alpha", URL: mustURL(t, alpha.URL)},
		{Alias: "beta", URL: mustURL(t, beta.URL)},
	}})
	require.NoError(t, err)
	client := newAggregateIssueClient(t, aggregate)

	sourceIssues, err := client.ListIssues(t.Context(), connect.NewRequest(&v1.ListIssuesRequest{
		Scope: &v1.BoardScope{
			Source:    &v1.SourceRef{SourceId: "alpha", StoreLineageId: "lineage-alpha"},
			Selection: &v1.BoardScope_AllBoards{AllBoards: &v1.AllBoards{}},
		},
		Sort: v1.IssueSort_ISSUE_SORT_TITLE,
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"Alpha A", "Alpha B"}, issueTitles(sourceIssues.Msg.GetIssues()))
	require.Len(t, alphaScopes, 1)
	_, allBoards := alphaScopes[0].GetSelection().(*v1.BoardScope_AllBoards)
	assert.True(t, allBoards)
	assert.Equal(t, 0, betaCalls)

	projectIssues, err := client.ListIssues(t.Context(), connect.NewRequest(&v1.ListIssuesRequest{
		Scope: &v1.BoardScope{
			Source:    &v1.SourceRef{SourceId: "alpha", StoreLineageId: "lineage-alpha"},
			Selection: &v1.BoardScope_ProjectId{ProjectId: "project-b"},
		},
	}))
	require.NoError(t, err)
	assert.Equal(t, []string{"Alpha B"}, issueTitles(projectIssues.Msg.GetIssues()))
	require.Len(t, alphaScopes, 2)
	assert.Equal(t, alphaBoardB.GetId(), alphaScopes[1].GetBoardId())
	assert.Equal(t, 0, betaCalls)
}

func TestAggregateReadsEmptyBoardPins(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-pins", ProjectId: "project", Name: "Pins"}
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
		pins: func(_ context.Context, request *connect.Request[v1.ListBoardPinsRequest]) (*connect.Response[v1.ListBoardPinsResponse], error) {
			assert.Equal(t, board.GetId(), request.Msg.GetBoardId())
			return connect.NewResponse(&v1.ListBoardPinsResponse{}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "pins", URL: mustURL(t, server.URL)}},
	})
	require.NoError(t, err)

	response, err := newAggregateIssueClient(t, aggregate).ListBoardPins(
		t.Context(),
		connect.NewRequest(&v1.ListBoardPinsRequest{BoardId: board.GetId()}),
	)
	require.NoError(t, err)
	assert.Empty(t, response.Msg.GetIssues())
}

func TestAggregateBoardPinsPreserveOrderAndQualifySource(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-pins", ProjectId: "project", Name: "Pins"}
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
		pins: func(_ context.Context, _ *connect.Request[v1.ListBoardPinsRequest]) (*connect.Response[v1.ListBoardPinsResponse], error) {
			return connect.NewResponse(&v1.ListBoardPinsResponse{Issues: []*v1.RelatedIssue{
				{Id: "issue-second", BoardId: board.GetId(), Title: "Second"},
				{Id: "issue-first", BoardId: board.GetId(), Title: "First"},
			}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{
		Sources: []SourceConfig{{Alias: "pins", URL: mustURL(t, server.URL)}},
	})
	require.NoError(t, err)

	response, err := newAggregateIssueClient(t, aggregate).ListBoardPins(
		t.Context(),
		connect.NewRequest(&v1.ListBoardPinsRequest{BoardId: board.GetId()}),
	)
	require.NoError(t, err)
	require.Len(t, response.Msg.GetIssues(), 2)
	assert.Equal(t, "issue-second", response.Msg.GetIssues()[0].GetId())
	assert.Equal(t, "issue-first", response.Msg.GetIssues()[1].GetId())
	for _, issue := range response.Msg.GetIssues() {
		assert.Equal(t, "pins", issue.GetSource().GetSourceId())
		assert.Equal(t, "lineage-primary", issue.GetSource().GetStoreLineageId())
	}
}

func TestAggregateBoardPinsUseSourceQualifiedBoardRoute(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-copied", ProjectId: "project", Name: "Copied"}
	firstBootstrap := sourceBootstrap(board, true)
	firstBootstrap.Sources[0].Source.StoreLineageId = "lineage-first"
	first := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: firstBootstrap,
		pins: func(context.Context, *connect.Request[v1.ListBoardPinsRequest]) (*connect.Response[v1.ListBoardPinsResponse], error) {
			return connect.NewResponse(&v1.ListBoardPinsResponse{Issues: []*v1.RelatedIssue{{
				Id: "first-issue", BoardId: board.GetId(), Title: "First",
			}}}), nil
		},
	})
	secondBootstrap := sourceBootstrap(board, true)
	secondBootstrap.Sources[0].Source.StoreLineageId = "lineage-second"
	second := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: secondBootstrap,
		pins: func(context.Context, *connect.Request[v1.ListBoardPinsRequest]) (*connect.Response[v1.ListBoardPinsResponse], error) {
			return connect.NewResponse(&v1.ListBoardPinsResponse{Issues: []*v1.RelatedIssue{{
				Id: "second-issue", BoardId: board.GetId(), Title: "Second",
			}}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{
		{Alias: "first", URL: mustURL(t, first.URL)},
		{Alias: "second", URL: mustURL(t, second.URL)},
	}})
	require.NoError(t, err)
	client := newAggregateIssueClient(t, aggregate)

	_, err = client.ListBoardPins(t.Context(), connect.NewRequest(
		&v1.ListBoardPinsRequest{BoardId: board.GetId()},
	))
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	response, err := client.ListBoardPins(t.Context(), connect.NewRequest(
		&v1.ListBoardPinsRequest{
			BoardId: board.GetId(),
			Source: &v1.SourceRef{
				SourceId: "second", StoreLineageId: "lineage-second",
			},
		},
	))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetIssues(), 1)
	assert.Equal(t, "second-issue", response.Msg.GetIssues()[0].GetId())
	assert.Equal(t, "second", response.Msg.GetIssues()[0].GetSource().GetSourceId())
}

func TestIssueDetailAndLogsCarrySourceQualifiedReferences(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-detail", ProjectId: "project", Name: "Detail"}
	bootstrap := sourceBootstrap(board, true)
	bootstrap.Sources[0].Source.StoreLineageId = "lineage-detail"
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrap,
		issue: func(_ context.Context, request *connect.Request[v1.GetIssueRequest]) (*connect.Response[v1.GetIssueResponse], error) {
			assert.Equal(t, "/source/detail/board", request.Msg.GetPresentation().GetRoutePrefix())
			return connect.NewResponse(&v1.GetIssueResponse{Issue: &v1.IssueDetail{
				Issue: &v1.IssueSummary{Id: "issue-detail", BoardId: board.GetId(), Title: "Detail"},
			}}), nil
		},
		logs: func(_ context.Context, request *connect.Request[v1.ListLogEntriesRequest]) (*connect.Response[v1.ListLogEntriesResponse], error) {
			assert.Equal(t, "/source/detail/board", request.Msg.GetPresentation().GetRoutePrefix())
			return connect.NewResponse(&v1.ListLogEntriesResponse{LogEntries: []*v1.LogEntry{{
				Id: "log-1", IssueId: "issue-detail",
				Payload: &v1.LogEntry_Post{Post: &v1.LogPost{Actor: "actor", Body: &v1.MarkdownContent{Source: "body"}}},
			}}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "detail", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)
	issueClient := newAggregateIssueClient(t, aggregate)
	source := &v1.SourceRef{SourceId: "detail", StoreLineageId: "lineage-detail"}
	boardID := board.GetId()
	detail, err := issueClient.GetIssue(t.Context(), connect.NewRequest(&v1.GetIssueRequest{
		IssueId: "issue-detail", Source: source, BoardId: &boardID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "detail", detail.Msg.GetIssue().GetIssue().GetSource().GetSourceId())

	logs, err := newAggregateRecordClient(t, aggregate).ListLogEntries(t.Context(), connect.NewRequest(&v1.ListLogEntriesRequest{
		IssueId: "issue-detail", Source: source, BoardId: &boardID,
	}))
	require.NoError(t, err)
	assert.Equal(t, "detail", logs.Msg.GetLogEntries()[0].GetSource().GetSourceId())
	assert.True(t, logs.Msg.GetAggregateStatus().GetComplete())
}

func TestAggregateReadsApprovalsAndRoutines(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-work", ProjectId: "project", Name: "Work"}
	bootstrap := sourceBootstrap(board, true)
	bootstrap.Sources[0].Source.StoreLineageId = "lineage-work"
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrap,
		issues: func(_ context.Context, _ *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error) {
			return connect.NewResponse(&v1.ListIssuesResponse{Issues: []*v1.IssueSummary{{
				Id: "routine-1", BoardId: board.GetId(), Title: "Routine", Type: v1.IssueType_ISSUE_TYPE_ROUTINE,
			}}}), nil
		},
		approvals: func(_ context.Context, _ *connect.Request[v1.ListActionableCheckpointsRequest]) (*connect.Response[v1.ListActionableCheckpointsResponse], error) {
			return connect.NewResponse(&v1.ListActionableCheckpointsResponse{Checkpoints: []*v1.ActionableCheckpoint{{
				Checkpoint: &v1.IssueSummary{Id: "approval-1", BoardId: board.GetId(), Title: "Approval", Type: v1.IssueType_ISSUE_TYPE_CHECKPOINT},
			}}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "work", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)

	routines, err := newAggregateIssueClient(t, aggregate).ListIssues(t.Context(), connect.NewRequest(&v1.ListIssuesRequest{
		Scope: &v1.BoardScope{
			Source:    &v1.SourceRef{SourceId: "work", StoreLineageId: "lineage-work"},
			Selection: &v1.BoardScope_AllBoards{AllBoards: &v1.AllBoards{}},
		},
		Types: []v1.IssueType{v1.IssueType_ISSUE_TYPE_ROUTINE},
	}))
	require.NoError(t, err)
	assert.Equal(t, "routine-1", routines.Msg.GetIssues()[0].GetId())
	assert.Equal(t, "work", routines.Msg.GetIssues()[0].GetSource().GetSourceId())

	approvals, err := privatev1connect.NewCheckpointServiceClient(
		&http.Client{Transport: handlerTransport{handler: aggregate.Binding().Handler}}, "http://aggregate.test",
	).ListActionableCheckpoints(t.Context(), connect.NewRequest(&v1.ListActionableCheckpointsRequest{
		Scope: &v1.BoardScope{Selection: &v1.BoardScope_AllSources{AllSources: &v1.AllSources{}}},
	}))
	require.NoError(t, err)
	assert.Equal(t, "approval-1", approvals.Msg.GetCheckpoints()[0].GetCheckpoint().GetId())
	assert.Equal(t, "work", approvals.Msg.GetCheckpoints()[0].GetCheckpoint().GetSource().GetSourceId())
}

func TestAggregateChangeStreamReportsDegradedSource(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-live", ProjectId: "project", Name: "Live"}
	bootstrap := sourceBootstrap(board, true)
	bootstrap.Sources[0].Source.StoreLineageId = "lineage-live"
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrap,
		changes: func(_ context.Context, _ *connect.Request[v1.WatchChangesRequest], stream *connect.ServerStream[v1.WatchChangesResponse]) error {
			if err := stream.Send(&v1.WatchChangesResponse{BoardId: board.GetId(), Revision: 7}); err != nil {
				return err
			}
			return errors.New("source stream ended")
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "live", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)
	aggregateHTTP := httptest.NewServer(aggregate.Binding().Handler)
	t.Cleanup(aggregateHTTP.Close)
	client := privatev1connect.NewChangeServiceClient(&http.Client{}, aggregateHTTP.URL)
	stream, err := client.WatchChanges(t.Context(), connect.NewRequest(&v1.WatchChangesRequest{
		Scope: &v1.BoardScope{Selection: &v1.BoardScope_BoardId{BoardId: board.GetId()}},
	}))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, stream.Close()) })
	require.True(t, stream.Receive())
	assert.Equal(t, "live", stream.Msg().GetSource().GetSourceId())
	require.True(t, stream.Receive())
	assert.Equal(t, v1.SourceHealth_SOURCE_HEALTH_DEGRADED, stream.Msg().GetHealth().GetHealth())
	assert.Equal(t, "live", stream.Msg().GetHealth().GetSource().GetSourceId())
}

type sourceHandler struct {
	privatev1connect.UnimplementedProjectServiceHandler
	privatev1connect.UnimplementedIssueServiceHandler
	privatev1connect.UnimplementedRecordServiceHandler
	privatev1connect.UnimplementedCheckpointServiceHandler
	privatev1connect.UnimplementedExecutionServiceHandler
	privatev1connect.UnimplementedChangeServiceHandler
	privatev1connect.UnimplementedAttachmentServiceHandler
	bootstrap   *v1.GetBootstrapResponse
	board       *v1.Board
	issues      func(context.Context, *connect.Request[v1.ListIssuesRequest]) (*connect.Response[v1.ListIssuesResponse], error)
	issue       func(context.Context, *connect.Request[v1.GetIssueRequest]) (*connect.Response[v1.GetIssueResponse], error)
	pins        func(context.Context, *connect.Request[v1.ListBoardPinsRequest]) (*connect.Response[v1.ListBoardPinsResponse], error)
	logs        func(context.Context, *connect.Request[v1.ListLogEntriesRequest]) (*connect.Response[v1.ListLogEntriesResponse], error)
	state       func(context.Context, *connect.Request[v1.GetStateRequest]) (*connect.Response[v1.GetStateResponse], error)
	attachments func(context.Context, *connect.Request[v1.ListAttachmentsRequest]) (*connect.Response[v1.ListAttachmentsResponse], error)
	approvals   func(context.Context, *connect.Request[v1.ListActionableCheckpointsRequest]) (*connect.Response[v1.ListActionableCheckpointsResponse], error)
	changes     func(context.Context, *connect.Request[v1.WatchChangesRequest], *connect.ServerStream[v1.WatchChangesResponse]) error
}

func (s *sourceHandler) GetBootstrap(
	context.Context,
	*connect.Request[v1.GetBootstrapRequest],
) (*connect.Response[v1.GetBootstrapResponse], error) {
	return connect.NewResponse(s.bootstrap), nil
}

func (s *sourceHandler) GetBoard(
	_ context.Context,
	request *connect.Request[v1.GetBoardRequest],
) (*connect.Response[v1.GetBoardResponse], error) {
	if s.board == nil || request.Msg.GetBoardId() != s.board.GetId() {
		return nil, connect.NewError(connect.CodeNotFound, nil)
	}
	return connect.NewResponse(&v1.GetBoardResponse{Board: s.board}), nil
}

func (s *sourceHandler) ListIssues(
	ctx context.Context,
	request *connect.Request[v1.ListIssuesRequest],
) (*connect.Response[v1.ListIssuesResponse], error) {
	if s.issues == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("issues not scripted"))
	}
	return s.issues(ctx, request)
}

func (s *sourceHandler) GetIssue(
	ctx context.Context,
	request *connect.Request[v1.GetIssueRequest],
) (*connect.Response[v1.GetIssueResponse], error) {
	if s.issue == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("issue not scripted"))
	}
	return s.issue(ctx, request)
}

func (s *sourceHandler) ListBoardPins(
	ctx context.Context,
	request *connect.Request[v1.ListBoardPinsRequest],
) (*connect.Response[v1.ListBoardPinsResponse], error) {
	if s.pins == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("pins not scripted"))
	}
	return s.pins(ctx, request)
}

func (s *sourceHandler) ListLogEntries(
	ctx context.Context,
	request *connect.Request[v1.ListLogEntriesRequest],
) (*connect.Response[v1.ListLogEntriesResponse], error) {
	if s.logs == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("logs not scripted"))
	}
	return s.logs(ctx, request)
}

func (s *sourceHandler) GetState(
	ctx context.Context,
	request *connect.Request[v1.GetStateRequest],
) (*connect.Response[v1.GetStateResponse], error) {
	if s.state == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("state not scripted"))
	}
	return s.state(ctx, request)
}

func (s *sourceHandler) ListAttachments(
	ctx context.Context,
	request *connect.Request[v1.ListAttachmentsRequest],
) (*connect.Response[v1.ListAttachmentsResponse], error) {
	if s.attachments == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("attachments not scripted"))
	}
	return s.attachments(ctx, request)
}

func (s *sourceHandler) ListActionableCheckpoints(
	ctx context.Context,
	request *connect.Request[v1.ListActionableCheckpointsRequest],
) (*connect.Response[v1.ListActionableCheckpointsResponse], error) {
	if s.approvals == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("approvals not scripted"))
	}
	return s.approvals(ctx, request)
}

func (s *sourceHandler) WatchChanges(
	ctx context.Context,
	request *connect.Request[v1.WatchChangesRequest],
	stream *connect.ServerStream[v1.WatchChangesResponse],
) error {
	if s.changes == nil {
		return connect.NewError(connect.CodeUnimplemented, errors.New("changes not scripted"))
	}
	return s.changes(ctx, request, stream)
}

func newSourceServer(
	t *testing.T,
	bootstrap *v1.GetBootstrapResponse,
	board *v1.Board,
) *httptest.Server {
	t.Helper()
	return newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: bootstrap, board: board,
	})
}

func newConfiguredSourceServer(t *testing.T, source *sourceHandler) *httptest.Server {
	t.Helper()
	return newConfiguredSourceServerWithRoutes(t, source, nil)
}

func newConfiguredSourceServerWithRoutes(
	t *testing.T,
	source *sourceHandler,
	routes http.Handler,
) *httptest.Server {
	t.Helper()
	_, handler := web.NewHandler(web.HandlerConfig{
		AccessMode:    web.AccessModeReadOnly,
		Project:       source,
		Configuration: new(privatev1connect.UnimplementedConfigurationServiceHandler),
		Information:   new(privatev1connect.UnimplementedInformationServiceHandler),
		Issue:         source,
		Planning:      new(privatev1connect.UnimplementedPlanningServiceHandler),
		Execution:     source,
		Checkpoint:    source,
		Record:        source,
		Change:        source,
		Dump:          new(privatev1connect.UnimplementedDumpServiceHandler),
		Mail:          new(privatev1connect.UnimplementedMailServiceHandler),
		Lease:         new(privatev1connect.UnimplementedLeaseServiceHandler),
		Attachment:    source,
	})
	serverHandler := http.Handler(handler)
	if routes != nil {
		serverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/board/") {
				routes.ServeHTTP(w, r)
				return
			}
			handler.ServeHTTP(w, r)
		})
	}
	server := httptest.NewServer(serverHandler)
	t.Cleanup(server.Close)
	return server
}

func sourceBootstrap(board *v1.BoardSummary, readOnly bool) *v1.GetBootstrapResponse {
	value := &v1.GetBootstrapResponse{
		AccessMode: v1.AccessMode_ACCESS_MODE_READ_ONLY,
		Sources: []*v1.SourceCatalogEntry{{
			Source:   &v1.SourceRef{StoreLineageId: "lineage-primary"},
			ReadOnly: readOnly,
		}},
	}
	if !readOnly {
		value.AccessMode = v1.AccessMode_ACCESS_MODE_READ_WRITE
	}
	if board != nil {
		value.Boards = []*v1.BoardSummary{board}
		value.Projects = []*v1.Project{{Id: board.GetProjectId(), Name: "Project"}}
	}
	return value
}

func newAggregateClient(t *testing.T, aggregate *Server) privatev1connect.ProjectServiceClient {
	t.Helper()
	binding := aggregate.Binding()
	client := &http.Client{Transport: handlerTransport{handler: binding.Handler}}
	return privatev1connect.NewProjectServiceClient(client, "http://aggregate.test")
}

func newAggregateIssueClient(t *testing.T, aggregate *Server) privatev1connect.IssueServiceClient {
	t.Helper()
	client := &http.Client{Transport: handlerTransport{handler: aggregate.Binding().Handler}}
	return privatev1connect.NewIssueServiceClient(client, "http://aggregate.test")
}

func newAggregateRecordClient(t *testing.T, aggregate *Server) privatev1connect.RecordServiceClient {
	t.Helper()
	client := &http.Client{Transport: handlerTransport{handler: aggregate.Binding().Handler}}
	return privatev1connect.NewRecordServiceClient(client, "http://aggregate.test")
}

func newAggregateAttachmentClient(t *testing.T, aggregate *Server) privatev1connect.AttachmentServiceClient {
	t.Helper()
	client := &http.Client{Transport: handlerTransport{handler: aggregate.Binding().Handler}}
	return privatev1connect.NewAttachmentServiceClient(client, "http://aggregate.test")
}

func issueTitles(values []*v1.IssueSummary) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.GetTitle())
	}
	return result
}

type handlerTransport struct {
	handler http.Handler
}

func (t handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}

func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	require.NoError(t, err)
	return parsed
}
