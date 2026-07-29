package attachment

import (
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

func TestBlobStoreWriteChunkReplaysOnlyMatchingBytes(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID, err := domainattachment.NewUploadID("../../upload-1")
	require.NoError(t, err)

	offset, err := store.writeChunk(uploadID, 0, []byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, uint64(5), offset)

	offset, err = store.writeChunk(uploadID, 0, []byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, uint64(5), offset)

	_, err = store.writeChunk(uploadID, 0, []byte("HELLO"))
	assert.Error(t, err)

	offset, err = store.writeChunk(uploadID, 3, []byte("lo!"))
	require.NoError(t, err)
	assert.Equal(t, uint64(6), offset)
}

func TestBlobStoreWriteChunkRejectsBoundsAndGaps(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID := mustUploadID(t, "upload-1")

	_, err = store.writeChunk(uploadID, 1, []byte("gap"))
	assert.Error(t, err)

	_, err = store.writeChunk(
		uploadID,
		0,
		make([]byte, domainattachment.MaxChunkSizeBytes+1),
	)
	assert.Error(t, err)

	_, err = store.writeChunk(
		uploadID,
		uint64(math.MaxInt64),
		[]byte("too large"),
	)
	assert.Error(t, err)
}

func TestBlobStoreWritesBeyondDefaultAdmissionLimit(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID := mustUploadID(t, "upload-1")
	require.NoError(t, store.beginStaging(uploadID))
	require.NoError(t, os.Truncate(
		store.stagingPath(uploadID),
		int64(domainattachment.MaxAttachmentSizeBytes),
	))

	offset, err := store.writeChunk(
		uploadID,
		domainattachment.MaxAttachmentSizeBytes,
		[]byte("x"),
	)

	require.NoError(t, err)
	assert.Equal(t, domainattachment.MaxAttachmentSizeBytes+1, offset)
}

func TestBlobStorePublishesAndDeduplicatesVerifiedContent(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	body := []byte("engineering report")
	expectedSize := uint64(len(body))
	expectedDigest := digestFor(t, body)

	firstUpload := mustUploadID(t, "upload-1")
	_, err = store.writeChunk(firstUpload, 0, body)
	require.NoError(t, err)
	descriptor, err := store.publish(firstUpload, &expectedSize, &expectedDigest)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobDescriptor{
		Digest: expectedDigest, SizeBytes: expectedSize,
	}, descriptor)

	availability, err := store.inspect(descriptor)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobAvailabilityPresentUnverified, availability)

	reader, availability, err := store.openVerified(descriptor)
	require.NoError(t, err)
	require.NotNil(t, reader)
	assert.Equal(t, domainattachment.BlobAvailabilityVerified, availability)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, body, got)
	require.NoError(t, reader.Close())
	_, err = reader.Read(make([]byte, 1))
	assert.Error(t, err)

	secondUpload := mustUploadID(t, "upload-2")
	_, err = store.writeChunk(secondUpload, 0, body)
	require.NoError(t, err)
	deduplicated, err := store.publish(secondUpload, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, descriptor, deduplicated)
}

func TestBlobStorePublishRequiresContentDirectorySync(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	body := []byte("durable")
	uploadID := mustUploadID(t, "upload-1")
	_, err = store.writeChunk(uploadID, 0, body)
	require.NoError(t, err)

	var syncCalls int
	store.syncDirectory = func(path string) error {
		assert.Equal(t, store.contentDir(), path)
		syncCalls++
		return assert.AnError
	}
	_, err = store.publish(uploadID, nil, nil)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 1, syncCalls)

	descriptor := domainattachment.BlobDescriptor{
		Digest: digestFor(t, body), SizeBytes: uint64(len(body)),
	}
	availability, err := store.inspect(descriptor)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobAvailabilityPresentUnverified, availability)

	store.syncDirectory = syncDirectory
	published, err := store.publish(uploadID, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, descriptor, published)
	remaining, err := store.collectExpiredStaging(
		[]domainattachment.UploadID{uploadID},
		true,
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 0, Bytes: 0}, remaining)
}

func TestBlobStorePublishChecksExpectedDescriptor(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	body := []byte("artifact")
	uploadID := mustUploadID(t, "upload-1")
	_, err = store.writeChunk(uploadID, 0, body)
	require.NoError(t, err)

	wrongSize := uint64(len(body) + 1)
	_, err = store.publish(uploadID, &wrongSize, nil)
	assert.Error(t, err)

	wrongDigest := digestFor(t, []byte("wrong"))
	_, err = store.publish(uploadID, nil, &wrongDigest)
	assert.Error(t, err)

	expectedSize := uint64(len(body))
	expectedDigest := digestFor(t, body)
	descriptor, err := store.publish(uploadID, &expectedSize, &expectedDigest)
	require.NoError(t, err)
	assert.Equal(t, expectedDigest, descriptor.Digest)
}

func TestBlobStorePublishesEmptyStaging(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID := mustUploadID(t, "empty-upload")
	require.NoError(t, store.beginStaging(uploadID))
	require.NoError(t, store.beginStaging(uploadID))

	expectedSize := uint64(0)
	expectedDigest := digestFor(t, nil)
	descriptor, err := store.publish(uploadID, &expectedSize, &expectedDigest)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobDescriptor{
		Digest: expectedDigest, SizeBytes: 0,
	}, descriptor)
}

func TestBlobStoreBeginStagingRequiresDirectorySync(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID := mustUploadID(t, "upload-1")

	var syncCalls int
	store.syncDirectory = func(path string) error {
		assert.Equal(t, store.stagingDir(), path)
		syncCalls++
		return assert.AnError
	}
	err = store.beginStaging(uploadID)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Equal(t, 1, syncCalls)

	store.syncDirectory = syncDirectory
	require.NoError(t, store.beginStaging(uploadID))
}

func TestBlobStoreObservesMissingAndCorruptContent(t *testing.T) {
	t.Run("Missing", func(t *testing.T) {
		store, err := newBlobStore(t.TempDir())
		require.NoError(t, err)
		descriptor := domainattachment.BlobDescriptor{
			Digest: digestFor(t, []byte("missing")), SizeBytes: 7,
		}

		availability, err := store.inspect(descriptor)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.BlobAvailabilityMissing, availability)
		reader, availability, err := store.openVerified(descriptor)
		require.NoError(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, domainattachment.BlobAvailabilityMissing, availability)
	})

	t.Run("SizeMismatch", func(t *testing.T) {
		store, descriptor := publishBody(t, []byte("expected"))
		require.NoError(t, os.Truncate(store.contentPath(descriptor.Digest), 1))

		availability, err := store.inspect(descriptor)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.BlobAvailabilitySizeMismatch, availability)
		reader, availability, err := store.openVerified(descriptor)
		require.NoError(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, domainattachment.BlobAvailabilitySizeMismatch, availability)
	})

	t.Run("DigestMismatchIsNotOverwritten", func(t *testing.T) {
		body := []byte("expected")
		store, descriptor := publishBody(t, body)
		require.NoError(t, os.WriteFile(
			store.contentPath(descriptor.Digest),
			[]byte("corrupt!"),
			0o600,
		))

		reader, availability, err := store.openVerified(descriptor)
		require.NoError(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, domainattachment.BlobAvailabilityDigestMismatch, availability)

		uploadID := mustUploadID(t, "replacement")
		_, err = store.writeChunk(uploadID, 0, body)
		require.NoError(t, err)
		_, err = store.publish(uploadID, nil, nil)
		assert.Error(t, err)

		reader, availability, err = store.openVerified(descriptor)
		require.NoError(t, err)
		assert.Nil(t, reader)
		assert.Equal(t, domainattachment.BlobAvailabilityDigestMismatch, availability)
	})
}

func TestBlobStoreCollectsSelectedExpiredStaging(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	expiredOne := mustUploadID(t, "expired-1")
	expiredTwo := mustUploadID(t, "expired-2")
	active := mustUploadID(t, "active")
	_, err = store.writeChunk(expiredOne, 0, []byte("one"))
	require.NoError(t, err)
	_, err = store.writeChunk(expiredTwo, 0, []byte("four"))
	require.NoError(t, err)
	_, err = store.writeChunk(active, 0, []byte("active"))
	require.NoError(t, err)

	expired := []domainattachment.UploadID{
		expiredOne, expiredTwo, expiredOne, mustUploadID(t, "missing"),
	}
	dryRun, err := store.collectExpiredStaging(expired, true)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 2, Bytes: 7}, dryRun)

	collected, err := store.collectExpiredStaging(expired, false)
	require.NoError(t, err)
	assert.Equal(t, dryRun, collected)

	collected, err = store.collectExpiredStaging(expired, false)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 0, Bytes: 0}, collected)

	descriptor, err := store.publish(active, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, uint64(6), descriptor.SizeBytes)
}

func TestBlobStoreCollectsOnlyOrphanBlobs(t *testing.T) {
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	retained := publishToStore(t, store, "retained", []byte("keep"))
	orphanOne := publishToStore(t, store, "orphan-1", []byte("discard"))
	orphanTwo := publishToStore(t, store, "orphan-2", []byte("remove"))
	unmanagedPath := filepath.Join(store.contentDir(), "operator-note")
	require.NoError(t, os.WriteFile(unmanagedPath, []byte("leave intact"), 0o600))

	dryRun, err := store.collectOrphanBlobs(
		[]domainattachment.BlobDescriptor{retained, retained},
		true,
	)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.CollectionSummary{Count: 2, Bytes: 13}, dryRun)

	collected, err := store.collectOrphanBlobs(
		[]domainattachment.BlobDescriptor{retained},
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, dryRun, collected)

	availability, err := store.inspect(retained)
	require.NoError(t, err)
	assert.Equal(t, domainattachment.BlobAvailabilityPresentUnverified, availability)
	for _, orphan := range []domainattachment.BlobDescriptor{orphanOne, orphanTwo} {
		availability, err := store.inspect(orphan)
		require.NoError(t, err)
		assert.Equal(t, domainattachment.BlobAvailabilityMissing, availability)
	}
	unmanaged, err := os.ReadFile(unmanagedPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("leave intact"), unmanaged)
}

func publishBody(t *testing.T, body []byte) (*blobStore, domainattachment.BlobDescriptor) {
	t.Helper()
	store, err := newBlobStore(t.TempDir())
	require.NoError(t, err)
	uploadID := mustUploadID(t, "upload-1")
	_, err = store.writeChunk(uploadID, 0, body)
	require.NoError(t, err)
	descriptor, err := store.publish(uploadID, nil, nil)
	require.NoError(t, err)
	return store, descriptor
}

func publishToStore(
	t *testing.T,
	store *blobStore,
	upload string,
	body []byte,
) domainattachment.BlobDescriptor {
	t.Helper()
	uploadID := mustUploadID(t, upload)
	_, err := store.writeChunk(uploadID, 0, body)
	require.NoError(t, err)
	descriptor, err := store.publish(uploadID, nil, nil)
	require.NoError(t, err)
	return descriptor
}

func mustUploadID(t *testing.T, value string) domainattachment.UploadID {
	t.Helper()
	id, err := domainattachment.NewUploadID(value)
	require.NoError(t, err)
	return id
}

func digestFor(t *testing.T, body []byte) domainattachment.Digest {
	t.Helper()
	digest, err := domainattachment.NewDigest(fmt.Sprintf("sha256:%x", sha256.Sum256(body)))
	require.NoError(t, err)
	return digest
}
