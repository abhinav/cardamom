package boardhistory

import (
	"bytes"
	"encoding/binary"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

func eventSequence(events ...Event) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for _, event := range events {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func partReader(parts [][]byte) func(int64) ([]byte, error) {
	return func(index int64) ([]byte, error) {
		if index < 0 || index >= int64(len(parts)) {
			return nil, errors.New("missing part")
		}
		return parts[index], nil
	}
}

func testHeader(t *testing.T, sequence uint64, previous Digest) Header {
	t.Helper()
	boardID := board.ID("board_test")
	writer := testWriter(t, 1)
	return Header{
		Board: boardID, Writer: writer, Sequence: sequence, Previous: previous,
	}
}

func testWriter(t *testing.T, value byte) board.WriterUID {
	t.Helper()
	writer, err := board.ParseWriterUID(bytes.Repeat([]byte{value}, 16))
	require.NoError(t, err)
	return writer
}

func testWriterNumber(t *testing.T, value uint64) board.WriterUID {
	t.Helper()
	var encoded [16]byte
	encoded[0] = 1
	binary.BigEndian.PutUint64(encoded[8:], value)
	writer, err := board.ParseWriterUID(encoded[:])
	require.NoError(t, err)
	return writer
}

func repeatedDigest(value byte) Digest {
	return Digest(bytes.Repeat([]byte{value}, 32))
}

func testIssueUID(t *testing.T) issue.UID {
	t.Helper()
	uid, err := issue.ParseUID(bytes.Repeat([]byte{2}, 16))
	require.NoError(t, err)
	return uid
}

func canonicalJSONForTest(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	canonical := jsontext.Value(encoded)
	if err := canonical.Canonicalize(); err != nil {
		return nil, err
	}
	return canonical, nil
}
