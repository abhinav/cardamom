package boardhistory

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
)

func TestEncodeMutation_streamsBoundedCanonicalParts(t *testing.T) {
	header := testHeader(t, 1, Digest{})
	issueUID := testIssueUID(t)
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{OccurredAt: at, Payload: &BoardOrigin{CreatedAt: at, Name: "Board"}},
		{OccurredAt: at, Payload: &IssueResult{
			IssueUID: issueUID,
			Body:     new(strings.Repeat("result ", 20_000)),
		}},
	}

	var parts [][]byte
	marker, err := EncodeMutation(header, eventSequence(events...), func(part Part) error {
		assert.Equal(t, int64(len(parts)), part.Index)
		assert.LessOrEqual(t, len(part.Bytes), mutationPartLimit)
		parts = append(parts, append([]byte(nil), part.Bytes...))
		return nil
	})
	require.NoError(t, err)
	assert.Greater(t, len(parts), 1)
	assert.Equal(t, uint64(len(events)), marker.EventCount())
	assert.Equal(t, int64(len(parts)), marker.PartCount())

	decoded, err := DecodeMarker(marker.Bytes())
	require.NoError(t, err)
	assert.Equal(t, marker.Hash(), decoded.Hash())

	var visited []Event
	for retained, err := range MutationEvents(decoded, partReader(parts)) {
		require.NoError(t, err)
		assert.Equal(t, uint64(len(visited)), retained.ID.Ordinal)
		visited = append(visited, retained.Event)
	}
	require.Len(t, visited, 2)
	assert.IsType(t, (*BoardOrigin)(nil), visited[0].Payload)
	result, ok := visited[1].Payload.(*IssueResult)
	require.True(t, ok)
	require.NotNil(t, result.Body)
	assert.Equal(t, strings.Repeat("result ", 20_000), *result.Body)
}

func TestEncodeMutation_transfersPartBytesToSink(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	event := Event{OccurredAt: at, Payload: &BoardNameSet{Name: "Board"}}
	var retained [][]byte
	marker, err := EncodeMutation(testHeader(t, 1, Digest{}), eventSequence(event), func(part Part) error {
		retained = append(retained, append([]byte(nil), part.Bytes...))
		clear(part.Bytes)
		return nil
	})
	require.NoError(t, err)
	var count int
	for record, err := range MutationEvents(marker, partReader(retained)) {
		require.NoError(t, err)
		assert.Equal(t, event, record.Event)
		count++
	}
	assert.Equal(t, 1, count)
}

func TestEncodeMutation_requiresContinuousWriterChain(t *testing.T) {
	_, err := EncodeMutation(
		testHeader(t, 2, Digest{}),
		eventSequence(Event{OccurredAt: time.Time{}, Payload: nil}),
		func(Part) error { return nil },
	)
	assert.ErrorContains(t, err, "predecessor")

	previous := Digest{1}
	_, err = EncodeMutation(
		testHeader(t, 2, previous),
		eventSequence(Event{OccurredAt: time.Time{}, Payload: nil}),
		func(Part) error { return nil },
	)
	assert.ErrorContains(t, err, "observed frontier")

	_, err = EncodeMutation(
		testHeader(t, 1, previous),
		eventSequence(Event{OccurredAt: time.Time{}, Payload: nil}),
		func(Part) error { return nil },
	)
	assert.ErrorContains(t, err, "must not name")
}

func TestEncodeMutation_canonicalizesFrontierWithoutAliasing(t *testing.T) {
	header := testHeader(t, 2, repeatedDigest(3))
	header.Frontier = []Tip{
		{Writer: testWriter(t, 2), Sequence: 9, Hash: repeatedDigest(4)},
		{Writer: header.Writer, Sequence: 1, Hash: header.Previous},
	}
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	var parts [][]byte
	marker, err := EncodeMutation(
		header,
		eventSequence(Event{
			OccurredAt: at,
			Payload:    &BoardNameSet{Name: "Board"},
		}),
		func(part Part) error {
			parts = append(parts, part.Bytes)
			return nil
		},
	)
	require.NoError(t, err)

	header.Frontier[0].Sequence = 100
	markerHeader := marker.Header()
	require.Len(t, markerHeader.Frontier, 2)
	assert.Equal(t, header.Writer, markerHeader.Frontier[0].Writer)
	assert.Equal(t, uint64(1), markerHeader.Frontier[0].Sequence)
	markerHeader.Frontier[0].Sequence = 200
	assert.Equal(t, uint64(1), marker.Header().Frontier[0].Sequence)
	for _, err := range MutationEvents(marker, partReader(parts)) {
		require.NoError(t, err)
	}
}

func TestEncodeMutation_rejectsDuplicateFrontierWriter(t *testing.T) {
	header := testHeader(t, 2, repeatedDigest(3))
	header.Frontier = []Tip{
		{Writer: header.Writer, Sequence: 1, Hash: header.Previous},
		{Writer: header.Writer, Sequence: 1, Hash: header.Previous},
	}
	_, err := EncodeMutation(
		header,
		eventSequence(Event{OccurredAt: time.Time{}, Payload: nil}),
		func(Part) error { return nil },
	)
	assert.ErrorContains(t, err, "unique writer")
}

func TestEncodeMutation_retainsStableV1Marker(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	boardID := board.ID("board_test")
	otherWriter := testWriter(t, 1)
	writer := testWriter(t, 2)
	previous := repeatedDigest(3)
	header := Header{
		Board: boardID, Writer: writer, Sequence: 2, Previous: previous,
		Frontier: []Tip{
			{Writer: otherWriter, Sequence: 7, Hash: repeatedDigest(4)},
			{Writer: writer, Sequence: 1, Hash: previous},
		},
	}
	event := Event{
		OccurredAt: at,
		Payload: &BoardDescriptionSet{
			Description: new("Notes"),
		},
	}
	encodedEvent, err := encodeEvent(event)
	require.NoError(t, err)

	var parts [][]byte
	marker, err := EncodeMutation(header, eventSequence(event), func(part Part) error {
		parts = append(parts, part.Bytes)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, parts, 1)

	const wantEvent = `{"kind":"board.description.set","occurred_at":"2026-09-16T10:00:00Z","value":{"description":{"present":true,"value":"Notes"}},"version":1}`
	const wantPayloadDigest = "4ffe34e297216712f3902897028e3b54664f58a880739f2b1ceffd1b056a81f8"
	const wantMarker = `{"board_id":"board_test","event_count":1,"frontier":[{"hash":"BAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQ=","sequence":7,"writer_uid":"AQEBAQEBAQEBAQEBAQEBAQ=="},{"hash":"AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=","sequence":1,"writer_uid":"AgICAgICAgICAgICAgICAg=="}],"part_count":1,"payload_hash":"T/404pchZxLzkCiXAo47VGZPWKiAc58rHO/9GwVqgfg=","previous":"AwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwM=","sequence":2,"version":1,"writer_uid":"AgICAgICAgICAgICAgICAg=="}`
	const wantMarkerDigest = "702ec63f64eeba1075332d7ae36195bee1cc77ef084079585e1cb97ce1ddd037"

	assert.Equal(t, wantEvent, string(encodedEvent))
	assert.Equal(t, wantEvent+"\n", string(parts[0]))
	assert.Equal(t, wantPayloadDigest, hex.EncodeToString(marker.PayloadHash().Bytes()))
	assert.Equal(t, wantMarker, string(marker.Bytes()))
	assert.Equal(t, wantMarkerDigest, hex.EncodeToString(marker.Hash().Bytes()))
}

func TestEncodeMutation_stopsOnSourceError(t *testing.T) {
	want := errors.New("read projection")
	events := func(yield func(Event, error) bool) {
		yield(Event{OccurredAt: time.Time{}, Payload: nil}, want)
	}
	_, err := EncodeMutation(testHeader(t, 1, Digest{}), events, func(Part) error {
		t.Fatal("part sink called")
		return nil
	})
	assert.ErrorIs(t, err, want)
}

func TestMutationEncoder_AppendEvent_flushesBoundedPartsBeforeFinish(t *testing.T) {
	issueUID := testIssueUID(t)
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	var parts []Part
	encoder, err := NewMutationEncoder(testHeader(t, 1, Digest{}), func(part Part) error {
		parts = append(parts, part)
		return nil
	})
	require.NoError(t, err)

	err = encoder.AppendEvent(Event{
		OccurredAt: at,
		Payload: &IssueResult{
			IssueUID: issueUID,
			Body:     new(strings.Repeat("result ", 20_000)),
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, parts)
	for _, part := range parts {
		assert.LessOrEqual(t, len(part.Bytes), mutationPartLimit)
	}
	marker, err := encoder.Finish()
	require.NoError(t, err)
	assert.Equal(t, int64(len(parts)), marker.PartCount())
}

func TestMutationEncoder_AppendEvent_retainsPartSinkFailure(t *testing.T) {
	want := errors.New("retain part")
	encoder, err := NewMutationEncoder(testHeader(t, 1, Digest{}), func(Part) error {
		return want
	})
	require.NoError(t, err)
	event := Event{
		OccurredAt: time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC),
		Payload: &IssueResult{
			IssueUID: testIssueUID(t),
			Body:     new(strings.Repeat("result ", 20_000)),
		},
	}

	firstErr := encoder.AppendEvent(event)
	require.ErrorIs(t, firstErr, want)
	assert.Equal(t, firstErr, encoder.AppendEvent(event))
	_, finishErr := encoder.Finish()
	assert.Equal(t, firstErr, finishErr)
}
