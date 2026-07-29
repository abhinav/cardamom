package attachment

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

const (
	blobsDirectory   = "blobs"
	contentDirectory = "sha256"
	stagingDirectory = "staging"
	maxBlobSizeBytes = uint64(math.MaxInt64)
)

var (
	errStagingOffsetConflict = errors.New("attachment staging offset conflict")
	errStagingChunkConflict  = errors.New("attachment staging chunk conflict")
)

// blobStore owns private filesystem storage for staged uploads and immutable
// content. Callers must serialize mutations for one upload or digest.
type blobStore struct {
	// root is the private blobs directory inside the resolved Cardamom store.
	root string

	// syncDirectory makes a published directory entry durable before callers
	// may commit metadata that references it.
	syncDirectory func(string) error // required
}

// newBlobStore binds blob storage to one resolved Cardamom store directory.
func newBlobStore(storeDir string) (*blobStore, error) {
	storeDir = strings.TrimSpace(storeDir)
	if storeDir == "" {
		return nil, errors.New("attachment store directory is required")
	}
	absoluteDir, err := filepath.Abs(storeDir)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment store directory %q: %w", storeDir, err)
	}
	return &blobStore{
		root:          filepath.Join(absoluteDir, blobsDirectory),
		syncDirectory: syncDirectory,
	}, nil
}

// beginStaging idempotently establishes an empty or existing upload staging
// file without truncating bytes accepted by an earlier invocation.
func (s *blobStore) beginStaging(uploadID domainattachment.UploadID) (err error) {
	if _, err := domainattachment.NewUploadID(uploadID.String()); err != nil {
		return err
	}
	if err := os.MkdirAll(s.stagingDir(), 0o700); err != nil {
		return fmt.Errorf("create attachment staging directory: %w", err)
	}
	file, err := os.OpenFile(s.stagingPath(uploadID), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open staged attachment: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect staged attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("staged attachment is not a regular file")
	}
	if info.Size() < 0 || uint64(info.Size()) > maxBlobSizeBytes {
		return fmt.Errorf(
			"staged attachment exceeds %d bytes",
			maxBlobSizeBytes,
		)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync staged attachment: %w", err)
	}
	if err := s.syncDirectory(s.stagingDir()); err != nil {
		return fmt.Errorf("sync attachment staging directory: %w", err)
	}
	return nil
}

// writeChunk appends content at expectedOffset or accepts an identical replay.
// It returns the complete staged size and syncs newly accepted bytes before
// returning them to the caller.
func (s *blobStore) writeChunk(
	uploadID domainattachment.UploadID,
	expectedOffset uint64,
	content []byte,
) (offset uint64, err error) {
	if _, err := domainattachment.NewUploadID(uploadID.String()); err != nil {
		return 0, err
	}
	if len(content) == 0 {
		return 0, errors.New("attachment chunk content required")
	}
	if len(content) > domainattachment.MaxChunkSizeBytes {
		return 0, fmt.Errorf(
			"attachment chunk exceeds %d bytes",
			domainattachment.MaxChunkSizeBytes,
		)
	}
	if expectedOffset > maxBlobSizeBytes ||
		uint64(len(content)) > maxBlobSizeBytes-expectedOffset {
		return 0, fmt.Errorf(
			"attachment upload exceeds %d bytes",
			maxBlobSizeBytes,
		)
	}

	if err := os.MkdirAll(s.stagingDir(), 0o700); err != nil {
		return 0, fmt.Errorf("create attachment staging directory: %w", err)
	}
	file, err := os.OpenFile(s.stagingPath(uploadID), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open staged attachment: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("inspect staged attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("staged attachment is not a regular file")
	}
	if info.Size() < 0 || uint64(info.Size()) > maxBlobSizeBytes {
		return 0, fmt.Errorf(
			"staged attachment exceeds %d bytes",
			maxBlobSizeBytes,
		)
	}
	currentSize := uint64(info.Size())
	if expectedOffset > currentSize {
		return 0, fmt.Errorf(
			"%w: "+
				"attachment chunk offset %d does not match staged size %d",
			errStagingOffsetConflict,
			expectedOffset,
			currentSize,
		)
	}

	// A replay may overlap bytes from a prior partial or complete write. Compare
	// the overlap before appending so a retry can never rewrite staged content.
	overlapSize := min(currentSize-expectedOffset, uint64(len(content)))
	if overlapSize > 0 {
		overlap := make([]byte, overlapSize)
		if _, err := file.ReadAt(overlap, int64(expectedOffset)); err != nil {
			return 0, fmt.Errorf("read staged attachment replay: %w", err)
		}
		if !bytes.Equal(overlap, content[:overlapSize]) {
			return 0, fmt.Errorf(
				"%w: attachment chunk conflicts with staged bytes at offset %d",
				errStagingChunkConflict,
				expectedOffset,
			)
		}
	}
	if overlapSize == uint64(len(content)) {
		return currentSize, nil
	}

	remainder := content[overlapSize:]
	written, err := file.WriteAt(remainder, int64(currentSize))
	if err != nil {
		return 0, fmt.Errorf("write staged attachment: %w", err)
	}
	if written != len(remainder) {
		return 0, io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return 0, fmt.Errorf("sync staged attachment: %w", err)
	}
	return currentSize + uint64(written), nil
}

func (s *blobStore) stagingDir() string {
	return filepath.Join(s.root, stagingDirectory)
}

func (s *blobStore) stagingPath(uploadID domainattachment.UploadID) string {
	name := base64.RawURLEncoding.EncodeToString([]byte(uploadID.String()))
	return filepath.Join(s.stagingDir(), name)
}

func (s *blobStore) contentDir() string {
	return filepath.Join(s.root, contentDirectory)
}

func (s *blobStore) contentPath(digest domainattachment.Digest) string {
	const prefix = "sha256:"
	return filepath.Join(s.contentDir(), strings.TrimPrefix(digest.String(), prefix))
}
