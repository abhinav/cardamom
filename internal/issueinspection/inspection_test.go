package issueinspection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestInspector_ReadIssue_markerNeedsNoBoard(t *testing.T) {
	marker := &issue.ReferenceMarker{ID: "card-old", Candidates: []issue.MarkerCandidate{}}
	markers := &testMarkerReader{marker: marker}
	reader := &testIssueReader{}
	inspector := Inspector{Markers: markers, Issues: reader}
	result, err := inspector.ReadIssue(t.Context(), issue.ReadRequest{IssueID: "card-old"})
	require.NoError(t, err)
	assert.Same(t, marker, result.Marker)
	assert.Nil(t, result.Issue)
	assert.False(t, reader.called)
}

func TestInspector_ReadIssue_ordinaryIssueKeepsRequestedContext(t *testing.T) {
	request := issue.ReadRequest{IssueID: "card-current", ContextDepth: new(2)}
	view := LocatedIssue{BoardID: "board-current", View: attachment.IssueView{
		Issue: issue.View{Detail: issue.Detail{Issue: issue.Issue{ID: "card-current"}}}, Attachments: nil,
	}}
	markers := &testMarkerReader{}
	reader := &testIssueReader{view: view}
	inspector := Inspector{Markers: markers, Issues: reader}
	result, err := inspector.ReadIssue(t.Context(), request)
	require.NoError(t, err)
	assert.Equal(t, &view, result.Issue)
	assert.Nil(t, result.Marker)
	assert.Equal(t, request, reader.request)
}

func TestInspector_ReadIssue_producerKeyDoesNotBecomeMarker(t *testing.T) {
	markers := &testMarkerReader{marker: &issue.ReferenceMarker{ID: "card-old", Candidates: nil}}
	reader := &testIssueReader{}
	inspector := Inspector{Markers: markers, Issues: reader}
	_, err := inspector.ReadIssue(t.Context(), issue.ReadRequest{Key: "card-old"})
	require.NoError(t, err)
	assert.False(t, markers.called)
	assert.Equal(t, "card-old", reader.request.Key)
}

type testMarkerReader struct {
	marker *issue.ReferenceMarker
	called bool
}

func (r *testMarkerReader) ReadMarker(context.Context, issue.ID) (*issue.ReferenceMarker, error) {
	r.called = true
	return r.marker, nil
}

type testIssueReader struct {
	view    LocatedIssue
	request issue.ReadRequest
	called  bool
}

func (r *testIssueReader) ReadIssue(_ context.Context, request issue.ReadRequest) (LocatedIssue, error) {
	r.called = true
	r.request = request
	return r.view, nil
}
