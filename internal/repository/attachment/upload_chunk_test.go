package attachment

import (
	"bytes"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryWriteChunkReplaysAndRejectsConflicts(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{2}, 128))
	upload := fixture.begin(t, "captain", nil, nil)

	written, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: []byte("hello"),
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(5), written.AcceptedOffset)

	replayed, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: []byte("hello"),
	})
	require.NoError(t, err)
	assert.Equal(t, written, replayed)

	partial, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 3, Content: []byte("lo!"),
	})
	require.NoError(t, err)
	assert.Equal(t, uint64(6), partial.AcceptedOffset)

	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: []byte("HELLO"),
	})
	assert.ErrorIs(t, err, domainattachment.ErrUploadChunkConflict)

	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: 7, Content: []byte("gap"),
	})
	assert.ErrorIs(t, err, domainattachment.ErrUploadOffsetConflict)

	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("commander"), UploadID: upload.ID,
		ExpectedOffset: 6, Content: []byte("!"),
	})
	assert.ErrorIs(t, err, domainattachment.ErrUploadActorConflict)
}

func TestRepositoryWriteChunkRejectsOffsetOverflow(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{2}, 128))
	upload := fixture.begin(t, "captain", nil, nil)

	_, err := fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("captain"), UploadID: upload.ID,
		ExpectedOffset: math.MaxUint64, Content: []byte("overflow"),
	})

	assert.ErrorIs(t, err, domainattachment.ErrUploadDescriptorMismatch)
}

func TestRepositorySerializesCompetingUploadWriters(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{11}, 128))
	upload := fixture.begin(t, "captain", nil, nil)
	secondStore, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(fixture.directory, "cardamom.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, secondStore.Close()) })
	secondRepository, err := New(secondStore, Config{
		StoreDirectory: fixture.directory,
		Clock:          fixture.clock,
		Entropy:        bytes.NewReader(bytes.Repeat([]byte{12}, 128)),
	})
	require.NoError(t, err)
	secondService := domainattachment.NewService(domainattachment.ServiceConfig{
		Repository: secondRepository,
	})

	results := make(chan error, 2)
	for _, operation := range []struct {
		service *domainattachment.Service
		content string
	}{
		{service: fixture.service, content: "alpha"},
		{service: secondService, content: "bravo"},
	} {
		go func() {
			_, err := operation.service.WriteChunk(
				t.Context(),
				domainattachment.WriteChunkRequest{
					Invocation: domainattachment.NewInvocation("captain"),
					UploadID:   upload.ID, ExpectedOffset: 0,
					Content: []byte(operation.content),
				},
			)
			results <- err
		}()
	}
	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, domainattachment.ErrUploadChunkConflict) {
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)
}
