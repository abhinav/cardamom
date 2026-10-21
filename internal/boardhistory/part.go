package boardhistory

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
)

const mutationPartLimit = 64 * 1024

// Part is one segment of the concatenated mutation event stream.
// A boundary may split an event or a UTF-8 sequence; decoding requires reading
// parts in Index order. The terminal Marker seals the complete stream.
type Part struct {
	// Index is the zero-based position in the mutation payload.
	Index int64

	// Bytes contains between 1 byte and 64 KiB of the event stream.
	// The encoder transfers this slice to the sink and does not reuse it.
	Bytes []byte // required
}

// partWriter emits every full part immediately and retains at most one partial
// part. An emit failure leaves earlier parts uncommitted until a Marker exists.
type partWriter struct {
	emit   func(Part) error
	digest hash.Hash
	index  int64
	buffer []byte
}

var _ io.StringWriter = (*partWriter)(nil)

func newPartWriter(emit func(Part) error) *partWriter {
	return &partWriter{emit: emit, digest: sha256.New()}
}

func (w *partWriter) Write(value []byte) (int, error) {
	written := 0
	for len(value) > 0 {
		remaining := mutationPartLimit - len(w.buffer)
		count := min(remaining, len(value))
		w.buffer = append(w.buffer, value[:count]...)
		value = value[count:]
		written += count
		if len(w.buffer) == mutationPartLimit {
			if err := w.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *partWriter) WriteString(value string) (int, error) {
	written := 0
	for len(value) > 0 {
		remaining := mutationPartLimit - len(w.buffer)
		count := min(remaining, len(value))
		w.buffer = append(w.buffer, value[:count]...)
		value = value[count:]
		written += count
		if len(w.buffer) == mutationPartLimit {
			if err := w.flush(); err != nil {
				return written, err
			}
		}
	}
	return written, nil
}

func (w *partWriter) flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	if w.index == math.MaxInt64 {
		return errors.New("mutation part count exceeds persistence range")
	}
	part := append([]byte(nil), w.buffer...)
	_, _ = w.digest.Write(part)
	if err := w.emit(Part{Index: w.index, Bytes: part}); err != nil {
		return err
	}
	w.index++
	w.buffer = w.buffer[:0]
	return nil
}

func (w *partWriter) finish() (int64, Digest, error) {
	if err := w.flush(); err != nil {
		return 0, Digest{}, err
	}
	var digest Digest
	copy(digest[:], w.digest.Sum(nil))
	return w.index, digest, nil
}

// payloadReader verifies the marker's payload digest only after the consumer
// reaches the end of every declared part.
type payloadReader struct {
	marker *Marker
	read   func(int64) ([]byte, error)
	digest hash.Hash
	index  int64
	buffer []byte
}

var _ io.Reader = (*payloadReader)(nil)

func newPayloadReader(marker *Marker, read func(int64) ([]byte, error)) (*payloadReader, error) {
	if marker == nil {
		return nil, errors.New("mutation marker required")
	}
	if read == nil {
		return nil, errors.New("mutation part reader required")
	}
	if len(marker.bytes) == 0 {
		return nil, errors.New("mutation marker is uninitialized")
	}
	return &payloadReader{marker: marker, read: read, digest: sha256.New()}, nil
}

func (r *payloadReader) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	for len(r.buffer) == 0 {
		if r.index == r.marker.partCount {
			if !bytes.Equal(r.digest.Sum(nil), r.marker.payloadHash[:]) {
				return 0, errors.New("mutation payload digest mismatch")
			}
			return 0, io.EOF
		}
		part, err := r.read(r.index)
		if err != nil {
			return 0, fmt.Errorf("read mutation part %d: %w", r.index, err)
		}
		if len(part) == 0 || len(part) > mutationPartLimit {
			return 0, fmt.Errorf("mutation part %d has invalid size", r.index)
		}
		r.buffer = part
		r.index++
	}
	count := copy(destination, r.buffer)
	_, _ = r.digest.Write(r.buffer[:count])
	r.buffer = r.buffer[count:]
	return count, nil
}
