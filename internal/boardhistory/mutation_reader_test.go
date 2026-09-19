package boardhistory

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutationEvents_rejectsTamperedPayload(t *testing.T) {
	header := testHeader(t, 1, Digest{})
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	var parts [][]byte
	marker, err := EncodeMutation(
		header,
		eventSequence(Event{
			OccurredAt: at,
			Payload:    &BoardOrigin{CreatedAt: at, Name: "Board"},
		}),
		func(part Part) error {
			parts = append(parts, append([]byte(nil), part.Bytes...))
			return nil
		},
	)
	require.NoError(t, err)
	parts[0][len(parts[0])-2] ^= 1

	var readErr error
	for _, err := range MutationEvents(marker, partReader(parts)) {
		if err != nil {
			readErr = err
		}
	}
	assert.ErrorContains(t, readErr, "mutation")
}

func TestMutationEvents_rejectsNilMarker(t *testing.T) {
	var errors int
	for _, err := range MutationEvents(nil, func(int64) ([]byte, error) {
		t.Fatal("part reader called")
		return nil, nil
	}) {
		assert.ErrorContains(t, err, "marker required")
		errors++
	}
	assert.Equal(t, 1, errors)
}

func TestMutationEvents_stopsAfterConsumerBreak(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{OccurredAt: at, Payload: &BoardNameSet{Name: "Board"}},
		{OccurredAt: at, Payload: &IssueResult{
			IssueUID: testIssueUID(t),
			Body:     new(strings.Repeat("result", mutationPartLimit)),
		}},
	}
	var parts [][]byte
	marker, err := EncodeMutation(testHeader(t, 1, Digest{}), eventSequence(events...), func(part Part) error {
		parts = append(parts, part.Bytes)
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, len(parts), 1)

	var reads, yielded int
	for retained, err := range MutationEvents(marker, func(index int64) ([]byte, error) {
		reads++
		require.Zero(t, index, "early stop must not read the next part")
		return parts[index], nil
	}) {
		require.NoError(t, err)
		assert.Equal(t, events[0], retained.Event)
		assert.Equal(t, marker.Header().Writer, retained.ID.Writer)
		assert.Equal(t, marker.Header().Sequence, retained.ID.Sequence)
		assert.Zero(t, retained.ID.Ordinal)
		yielded++
		break
	}
	assert.Equal(t, 1, reads)
	assert.Equal(t, 1, yielded)
}

func TestMutationEvents_reportsTerminalVerificationFailure(t *testing.T) {
	at := time.Date(2026, time.September, 16, 10, 0, 0, 0, time.UTC)
	event := Event{OccurredAt: at, Payload: &BoardNameSet{Name: "Board"}}
	var parts [][]byte
	marker, err := EncodeMutation(testHeader(t, 1, Digest{}), eventSequence(event), func(part Part) error {
		parts = append(parts, part.Bytes)
		return nil
	})
	require.NoError(t, err)

	t.Run("DigestAfterValidEvent", func(t *testing.T) {
		// JSON whitespace preserves the event but changes the committed payload.
		changed := append([]byte(nil), parts[0]...)
		changed[len(changed)-1] = ' '
		var events, failures int
		for retained, err := range MutationEvents(marker, partReader([][]byte{changed})) {
			if err != nil {
				assert.ErrorContains(t, err, "payload digest mismatch")
				failures++
				continue
			}
			assert.Equal(t, event, retained.Event)
			events++
		}
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, failures)
	})

	t.Run("CountAfterValidEvent", func(t *testing.T) {
		encoded := strings.Replace(string(marker.Bytes()), `"event_count":1`, `"event_count":2`, 1)
		marker, err := DecodeMarker([]byte(encoded))
		require.NoError(t, err)
		var events, failures int
		for retained, err := range MutationEvents(marker, partReader(parts)) {
			if err != nil {
				assert.ErrorContains(t, err, "contains 1 events; marker declares 2")
				failures++
				continue
			}
			assert.Equal(t, event, retained.Event)
			events++
		}
		assert.Equal(t, 1, events)
		assert.Equal(t, 1, failures)
	})

	t.Run("ReadFailure", func(t *testing.T) {
		want := errors.New("read retained part")
		var failures int
		for _, err := range MutationEvents(marker, func(int64) ([]byte, error) {
			return nil, want
		}) {
			assert.ErrorIs(t, err, want)
			failures++
		}
		assert.Equal(t, 1, failures)
	})
}

func TestMutationEvents_rejectsUninitializedMarker(t *testing.T) {
	var failures int
	for _, err := range MutationEvents(new(Marker), func(int64) ([]byte, error) {
		t.Fatal("part reader called")
		return nil, nil
	}) {
		assert.ErrorContains(t, err, "marker is uninitialized")
		failures++
	}
	assert.Equal(t, 1, failures)
}
