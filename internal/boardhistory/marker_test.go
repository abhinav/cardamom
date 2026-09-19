package boardhistory

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
)

func TestDecodeMarker_rejectsInvalidFrontier(t *testing.T) {
	boardID := board.ID("board_test")
	writer := testWriter(t, 3)
	base := markerV1{
		Version: 1, BoardID: boardID.String(), WriterUID: writer.Bytes(),
		Sequence: 1, Previous: Digest{}.Bytes(), Frontier: []tipV1{}, PartCount: 1,
		EventCount: 1, PayloadHash: repeatedDigest(9).Bytes(),
	}
	encoded, err := encodeMarkerV1(base)
	require.NoError(t, err)
	nullFrontier := strings.Replace(
		string(encoded), `"frontier":[]`, `"frontier":null`, 1,
	)
	_, err = DecodeMarker([]byte(nullFrontier))
	assert.ErrorContains(t, err, "frontier array")

	tests := []struct {
		name     string
		frontier []tipV1
		want     string
	}{
		{
			name: "Unsorted",
			frontier: []tipV1{
				{WriterUID: testWriter(t, 2).Bytes(), Sequence: 1, Hash: repeatedDigest(2).Bytes()},
				{WriterUID: testWriter(t, 1).Bytes(), Sequence: 1, Hash: repeatedDigest(1).Bytes()},
			},
			want: "sorted",
		},
		{
			name: "DuplicateWriter",
			frontier: []tipV1{
				{WriterUID: testWriter(t, 1).Bytes(), Sequence: 1, Hash: repeatedDigest(1).Bytes()},
				{WriterUID: testWriter(t, 1).Bytes(), Sequence: 2, Hash: repeatedDigest(2).Bytes()},
			},
			want: "unique writer",
		},
		{
			name: "ZeroWriter",
			frontier: []tipV1{
				{WriterUID: make([]byte, 16), Sequence: 1, Hash: repeatedDigest(1).Bytes()},
			},
			want: "frontier tip 0 writer",
		},
		{
			name: "ZeroSequence",
			frontier: []tipV1{
				{WriterUID: testWriter(t, 1).Bytes(), Hash: repeatedDigest(1).Bytes()},
			},
			want: "invalid frontier tip",
		},
		{
			name: "ZeroDigest",
			frontier: []tipV1{
				{WriterUID: testWriter(t, 1).Bytes(), Sequence: 1, Hash: Digest{}.Bytes()},
			},
			want: "invalid frontier tip",
		},
		{
			name: "OwnTipOnFirstMutation",
			frontier: []tipV1{
				{WriterUID: writer.Bytes(), Sequence: 1, Hash: repeatedDigest(1).Bytes()},
			},
			want: "must not observe its own tip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := base
			wire.Frontier = tt.frontier
			encoded, err := encodeMarkerV1(wire)
			require.NoError(t, err)
			_, err = DecodeMarker(encoded)
			assert.ErrorContains(t, err, tt.want)
		})
	}
}

func TestDecodeMarker_dispatchesUnknownVersionBeforeStrictShape(t *testing.T) {
	_, err := DecodeMarker([]byte(`{"future_member":true,"version":2}`))
	require.Error(t, err)
	assert.ErrorContains(t, err, "unsupported mutation marker version 2")
	assert.NotContains(t, err.Error(), "unknown")
}

func TestDecodeMarker_enforcesSizeBound(t *testing.T) {
	_, err := DecodeMarker(nil)
	assert.ErrorContains(t, err, "marker size")
	_, err = DecodeMarker(make([]byte, mutationMarkerLimit+1))
	assert.ErrorContains(t, err, "marker size")

	header := testHeader(t, 1, Digest{})
	header.Writer = testWriter(t, 0xff)
	for index := range 700 {
		header.Frontier = append(header.Frontier, Tip{
			Writer:   testWriterNumber(t, uint64(index+1)),
			Sequence: 1,
			Hash:     repeatedDigest(byte(index%255 + 1)),
		})
	}
	_, err = newMarker(header, 1, 1, repeatedDigest(9))
	assert.ErrorContains(t, err, "marker exceeds")
}

func TestEncodeMutation_retainsCanonicalFrontierSequence(t *testing.T) {
	// V1 canonical JSON rounds integer sequences beyond double precision.
	// The marker's structured header must describe the retained bytes.
	header := testHeader(t, 1, Digest{})
	header.Frontier = []Tip{{
		Writer: testWriter(t, 2), Sequence: 1<<53 + 1, Hash: repeatedDigest(9),
	}}
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	marker, err := EncodeMutation(header, eventSequence(Event{
		OccurredAt: at, Payload: &BoardNameSet{Name: "Board"},
	}), func(Part) error { return nil })
	require.NoError(t, err)
	decoded, err := DecodeMarker(marker.Bytes())
	require.NoError(t, err)
	assert.Equal(t, uint64(1<<53), marker.Header().Frontier[0].Sequence)
	assert.Equal(t, decoded.Header(), marker.Header())
}
