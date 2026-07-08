package attachment

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

// inspect reports the cheap local observation available without reading the
// complete blob.
func (s *blobStore) inspect(
	descriptor domainattachment.BlobDescriptor,
) (availability domainattachment.BlobAvailability, err error) {
	if err := descriptor.Validate(); err != nil {
		return 0, err
	}
	file, err := os.Open(s.contentPath(descriptor.Digest))
	if errors.Is(err, os.ErrNotExist) {
		return domainattachment.BlobAvailabilityMissing, nil
	}
	if err != nil {
		return 0, fmt.Errorf("open attachment blob: %w", err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	return inspectOpenFile(file, descriptor)
}

// openVerified verifies one read-only file descriptor, rewinds it, and
// transfers close ownership to the caller only when the descriptor matches.
func (s *blobStore) openVerified(
	descriptor domainattachment.BlobDescriptor,
) (io.ReadSeekCloser, domainattachment.BlobAvailability, error) {
	if err := descriptor.Validate(); err != nil {
		return nil, 0, err
	}
	file, err := os.Open(s.contentPath(descriptor.Digest))
	if errors.Is(err, os.ErrNotExist) {
		return nil, domainattachment.BlobAvailabilityMissing, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("open attachment blob: %w", err)
	}

	availability, err := verifyOpenFile(file, descriptor)
	if err != nil {
		return nil, 0, errors.Join(err, file.Close())
	}
	if availability != domainattachment.BlobAvailabilityVerified {
		return nil, availability, file.Close()
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, 0, errors.Join(
			fmt.Errorf("rewind verified attachment blob: %w", err),
			file.Close(),
		)
	}
	return &blobReader{file: file}, availability, nil
}

func inspectOpenFile(
	file *os.File,
	descriptor domainattachment.BlobDescriptor,
) (domainattachment.BlobAvailability, error) {
	info, err := file.Stat()
	if err != nil {
		return 0, fmt.Errorf("inspect attachment blob: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("attachment blob is not a regular file")
	}
	if info.Size() < 0 || uint64(info.Size()) != descriptor.SizeBytes {
		return domainattachment.BlobAvailabilitySizeMismatch, nil
	}
	return domainattachment.BlobAvailabilityPresentUnverified, nil
}

func verifyOpenFile(
	file *os.File,
	descriptor domainattachment.BlobDescriptor,
) (domainattachment.BlobAvailability, error) {
	availability, err := inspectOpenFile(file, descriptor)
	if err != nil || availability != domainattachment.BlobAvailabilityPresentUnverified {
		return availability, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("rewind attachment blob: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return 0, fmt.Errorf("hash attachment blob: %w", err)
	}
	if fmt.Sprintf("sha256:%x", hash.Sum(nil)) != descriptor.Digest.String() {
		return domainattachment.BlobAvailabilityDigestMismatch, nil
	}
	return domainattachment.BlobAvailabilityVerified, nil
}

// blobReader exposes read, seek, and close ownership without exposing the
// underlying file or its private path.
type blobReader struct {
	// file is the read-only verified descriptor owned until Close.
	file *os.File
}

func (r *blobReader) Read(data []byte) (int, error) {
	return r.file.Read(data)
}

func (r *blobReader) Seek(offset int64, whence int) (int64, error) {
	return r.file.Seek(offset, whence)
}

func (r *blobReader) Close() error {
	return r.file.Close()
}
