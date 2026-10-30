package aggregate

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
)

func TestAggregateRetainsMarkerOnOldBoardRoute(t *testing.T) {
	board := &v1.BoardSummary{Id: "board-old", ProjectId: "project", Name: "Original board"}
	server := newConfiguredSourceServer(t, &sourceHandler{
		bootstrap: sourceBootstrap(board, true),
		issue: func(_ context.Context, _ *connect.Request[v1.GetIssueRequest]) (*connect.Response[v1.GetIssueResponse], error) {
			return connect.NewResponse(&v1.GetIssueResponse{Marker: &v1.IssueReferenceMarker{
				Id: "card-retired", Candidates: []*v1.IssueReferenceCandidate{
					{Id: "card-first", BoardId: "board-other", Available: true},
					{Id: "card-second", BoardId: "board-absent"},
				},
			}}), nil
		},
	})
	aggregate, err := New(t.Context(), Config{Sources: []SourceConfig{{Alias: "primary", URL: mustURL(t, server.URL)}}})
	require.NoError(t, err)
	response, err := newAggregateIssueClient(t, aggregate).GetIssue(t.Context(), connect.NewRequest(&v1.GetIssueRequest{
		IssueId: "card-retired", BoardId: new(board.GetId()),
		Source: &v1.SourceRef{SourceId: "primary", StoreLineageId: "lineage-primary"},
	}))
	require.NoError(t, err)
	assert.Nil(t, response.Msg.GetIssue())
	require.Len(t, response.Msg.GetMarker().GetCandidates(), 2)
	for _, candidate := range response.Msg.GetMarker().GetCandidates() {
		assert.Equal(t, "primary", candidate.GetSource().GetSourceId())
	}
	assert.True(t, response.Msg.GetMarker().GetCandidates()[0].GetAvailable())
	assert.False(t, response.Msg.GetMarker().GetCandidates()[1].GetAvailable())
}
