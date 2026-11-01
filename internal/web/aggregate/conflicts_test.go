package aggregate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/gen/cardamom/private/v1/privatev1connect"
)

func TestConflictReadsPreserveSourceAndRejectAggregateResolution(t *testing.T) {
	source := &conflictSource{t: t}
	source.bootstrap = sourceBootstrap(&v1.BoardSummary{Id: "board-shared", ProjectId: "project-primary", Name: new("Board")}, false)
	mux := http.NewServeMux()
	path, handler := privatev1connect.NewProjectServiceHandler(source)
	mux.Handle(path, handler)
	path, handler = privatev1connect.NewConflictServiceHandler(source)
	mux.Handle(path, handler)
	remote := httptest.NewServer(mux)
	defer remote.Close()
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "remote", URL: mustURL(t, remote.URL)}}, Version: "conflict-test"})
	require.NoError(t, err)
	client := privatev1connect.NewConflictServiceClient(&http.Client{Transport: handlerTransport{handler: aggregate.Binding().Handler}}, "http://aggregate.test")
	scope := &v1.ConflictScope{BoardId: "board-shared", Source: &v1.SourceRef{SourceId: "remote"}}
	list, err := client.ListConflicts(t.Context(), connect.NewRequest(&v1.ListConflictsRequest{Scope: scope, After: "previous", Limit: 7}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Conflicts, 1)
	assert.Equal(t, "remote", list.Msg.Conflicts[0].Issues[0].GetSource().GetSourceId())
	assert.Equal(t, "next", list.Msg.Next)
	body, err := client.GetConflictAlternative(t.Context(), connect.NewRequest(&v1.GetConflictAlternativeRequest{Scope: scope, ConflictId: "reviewed", AlternativeId: "selected"}))
	require.NoError(t, err)
	assert.Equal(t, "remote", body.Msg.Value.Issues[0].Issue.GetSource().GetSourceId())
	assert.Equal(t, "retained", body.Msg.Value.GetText().GetText())
	_, err = client.ResolveConflict(t.Context(), connect.NewRequest(&v1.ResolveConflictRequest{Scope: scope, ConflictId: "reviewed", Choice: &v1.ResolveConflictRequest_Take{Take: "selected"}}))
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

// conflictSource is a protocol fixture: only bootstrap and the exercised
// conflict reads are served. Resolution remains unimplemented at the source.
type conflictSource struct {
	sourceHandler
	privatev1connect.UnimplementedConflictServiceHandler
	t *testing.T
}

func (s *conflictSource) ListConflicts(_ context.Context, request *connect.Request[v1.ListConflictsRequest]) (*connect.Response[v1.ListConflictsResponse], error) {
	assert.Equal(s.t, "board-shared", request.Msg.GetScope().GetBoardId())
	assert.Nil(s.t, request.Msg.GetScope().Source)
	assert.Equal(s.t, "previous", request.Msg.After)
	assert.Equal(s.t, uint32(7), request.Msg.Limit)
	return connect.NewResponse(&v1.ListConflictsResponse{Conflicts: []*v1.ConflictSummary{{Id: "reviewed", Issues: []*v1.RelatedIssue{{Id: "cm-issue", BoardId: "board-shared"}}}}, Next: "next"}), nil
}

func (s *conflictSource) GetConflictAlternative(_ context.Context, request *connect.Request[v1.GetConflictAlternativeRequest]) (*connect.Response[v1.GetConflictAlternativeResponse], error) {
	assert.Equal(s.t, "reviewed", request.Msg.ConflictId)
	assert.Equal(s.t, "selected", request.Msg.AlternativeId)
	assert.Nil(s.t, request.Msg.GetScope().Source)
	return connect.NewResponse(&v1.GetConflictAlternativeResponse{Value: &v1.ConflictValue{Value: &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: new("retained")}}, Issues: []*v1.ConflictIssue{{Uid: "uid", Issue: &v1.RelatedIssue{Id: "cm-issue", BoardId: "board-shared"}}}}}), nil
}
