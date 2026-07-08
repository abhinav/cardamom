package attachment

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	domainattachment "go.abhg.dev/cardamom/internal/attachment"
)

var errDescriptorMismatch = errors.New("attachment descriptor mismatch")

// publish verifies and atomically publishes one staged upload. An existing
// digest target is accepted only after its complete content is verified.
func (s *blobStore) publish(
	uploadID domainattachment.UploadID,
	expectedSize *uint64,
	expectedDigest *domainattachment.Digest,
) (domainattachment.BlobDescriptor, error) {
	descriptor, err := s.publishForCommit(
		uploadID,
		domainattachment.MaxAttachmentSizeBytes,
		expectedSize,
		expectedDigest,
	)
	if err != nil {
		return domainattachment.BlobDescriptor{}, err
	}
	if err := s.removeStaging(uploadID); err != nil {
		return domainattachment.BlobDescriptor{}, err
	}
	return descriptor, nil
}

// publishForCommit leaves staging intact until the caller commits metadata.
func (s *blobStore) publishForCommit(
	uploadID domainattachment.UploadID,
	maximumSizeBytes uint64,
	expectedSize *uint64,
	expectedDigest *domainattachment.Digest,
) (domainattachment.BlobDescriptor, error) {
	if _, err := domainattachment.NewUploadID(uploadID.String()); err != nil {
		return domainattachment.BlobDescriptor{}, err
	}
	if maximumSizeBytes == 0 || maximumSizeBytes > maxBlobSizeBytes {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"attachment maximum must be between 1 and %d bytes",
			maxBlobSizeBytes,
		)
	}
	if expectedSize != nil && *expectedSize > maximumSizeBytes {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"attachment expected size exceeds %d bytes",
			maximumSizeBytes,
		)
	}
	if expectedDigest != nil {
		digest, err := domainattachment.NewDigest(expectedDigest.String())
		if err != nil || digest != *expectedDigest {
			return domainattachment.BlobDescriptor{}, errors.New("valid attachment expected digest required")
		}
	}

	stagedPath := s.stagingPath(uploadID)
	staged, err := os.Open(stagedPath)
	if err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("open staged attachment for publication: %w", err)
	}
	descriptor, hashErr := descriptorFromFile(staged, maximumSizeBytes)
	closeErr := staged.Close()
	if err := errors.Join(hashErr, closeErr); err != nil {
		return domainattachment.BlobDescriptor{}, err
	}
	if expectedSize != nil && descriptor.SizeBytes != *expectedSize {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"%w: staged attachment size %d does not match expected size %d",
			errDescriptorMismatch,
			descriptor.SizeBytes,
			*expectedSize,
		)
	}
	if expectedDigest != nil && descriptor.Digest != *expectedDigest {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"%w: staged attachment digest %s does not match expected digest %s",
			errDescriptorMismatch,
			descriptor.Digest,
			*expectedDigest,
		)
	}

	if err := os.MkdirAll(s.contentDir(), 0o700); err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("create attachment content directory: %w", err)
	}
	target := s.contentPath(descriptor.Digest)
	if err := os.Link(stagedPath, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return domainattachment.BlobDescriptor{}, fmt.Errorf("publish attachment blob: %w", err)
		}
		reader, availability, verifyErr := s.openVerified(descriptor)
		if reader != nil {
			verifyErr = errors.Join(verifyErr, reader.Close())
		}
		if verifyErr != nil {
			return domainattachment.BlobDescriptor{}, verifyErr
		}
		if availability != domainattachment.BlobAvailabilityVerified {
			return domainattachment.BlobDescriptor{}, fmt.Errorf(
				"existing attachment blob %s is %s",
				descriptor.Digest,
				availability,
			)
		}
	}
	if err := s.syncDirectory(s.contentDir()); err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"sync attachment content directory: %w",
			err,
		)
	}
	return descriptor, nil
}

func (s *blobStore) removeStaging(uploadID domainattachment.UploadID) error {
	if err := os.Remove(s.stagingPath(uploadID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove published attachment staging: %w", err)
	}
	if err := s.syncDirectory(s.stagingDir()); err != nil {
		return fmt.Errorf("sync attachment staging directory: %w", err)
	}
	return nil
}

func descriptorFromFile(
	file *os.File,
	maximumSizeBytes uint64,
) (domainattachment.BlobDescriptor, error) {
	info, err := file.Stat()
	if err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("inspect staged attachment: %w", err)
	}
	if !info.Mode().IsRegular() {
		return domainattachment.BlobDescriptor{}, errors.New("staged attachment is not a regular file")
	}
	if info.Size() < 0 || uint64(info.Size()) > maximumSizeBytes {
		return domainattachment.BlobDescriptor{}, fmt.Errorf(
			"staged attachment exceeds %d bytes",
			maximumSizeBytes,
		)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("rewind staged attachment: %w", err)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("hash staged attachment: %w", err)
	}
	if size != info.Size() {
		return domainattachment.BlobDescriptor{}, errors.New("staged attachment changed while hashing")
	}
	digest, err := domainattachment.NewDigest(fmt.Sprintf("sha256:%x", hash.Sum(nil)))
	if err != nil {
		return domainattachment.BlobDescriptor{}, fmt.Errorf("construct staged attachment digest: %w", err)
	}
	return domainattachment.BlobDescriptor{
		Digest: digest, SizeBytes: uint64(size),
	}, nil
}

func syncDirectory(path string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
