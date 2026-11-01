package boardhistory

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestReferenceMarkerEventRetainsCrossBoardCandidates(t *testing.T) {
	other, err := issue.ParseUID(bytes.Repeat([]byte{3}, 16))
	require.NoError(t, err)
	event := Event{OccurredAt: time.Unix(1_700_000_000, 0).UTC(), Payload: &ReferenceMarker{
		PublicID: "card-old", Candidates: []ReferenceCandidate{
			{IssueUID: testIssueUID(t), BoardID: "board-a"},
			{IssueUID: other, BoardID: "board-absent"},
		},
	}}
	encoded, err := encodeEvent(event)
	require.NoError(t, err)
	decoded, err := decodeEvent(encoded)
	require.NoError(t, err)
	assert.Equal(t, event, decoded)

	event.Payload = &ReferenceMarker{PublicID: "card-old", Candidates: []ReferenceCandidate{
		{IssueUID: other, BoardID: "board-a"}, {IssueUID: other, BoardID: "board-b"},
	}}
	_, err = encodeEvent(event)
	assert.ErrorContains(t, err, "repeats an issue identity")
}
