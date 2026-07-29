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

// publishReader verifies reader against descriptor and idempotently publishes
// the content-addressed bytes.
func (s *blobStore) publishReader(
	descriptor domainattachment.BlobDescriptor,
	reader io.Reader,
) (err error) {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if reader == nil {
		return errors.New("attachment blob reader is required")
	}

	existing, availability, err := s.openVerified(descriptor)
	if err != nil {
		return err
	}
	if existing != nil {
		return existing.Close()
	}
	if availability != domainattachment.BlobAvailabilityMissing {
		return fmt.Errorf(
			"existing attachment blob %s is %s",
			descriptor.Digest,
			availability,
		)
	}

	if err := os.MkdirAll(s.contentDir(), 0o700); err != nil {
		return fmt.Errorf("create attachment content directory: %w", err)
	}
	temporary, err := os.CreateTemp(s.contentDir(), ".copy-*")
	if err != nil {
		return fmt.Errorf("create attachment blob temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		err = errors.Join(err, os.Remove(temporaryPath))
	}()

	hash := sha256.New()
	written, copyErr := io.CopyN(
		io.MultiWriter(temporary, hash),
		reader,
		int64(descriptor.SizeBytes),
	)
	if copyErr != nil {
		_ = temporary.Close()
		return fmt.Errorf("read attachment blob: %w", copyErr)
	}
	var extra [1]byte
	extraBytes, extraErr := reader.Read(extra[:])
	if extraErr != nil && !errors.Is(extraErr, io.EOF) {
		_ = temporary.Close()
		return fmt.Errorf("read attachment blob trailer: %w", extraErr)
	}
	if written != int64(descriptor.SizeBytes) || extraBytes != 0 {
		_ = temporary.Close()
		return fmt.Errorf(
			"%w: attachment blob size exceeds descriptor size %d",
			errDescriptorMismatch,
			descriptor.SizeBytes,
		)
	}
	digest := fmt.Sprintf("sha256:%x", hash.Sum(nil))
	if digest != descriptor.Digest.String() {
		_ = temporary.Close()
		return fmt.Errorf(
			"%w: attachment blob digest %s does not match descriptor digest %s",
			errDescriptorMismatch,
			digest,
			descriptor.Digest,
		)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync attachment blob: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close attachment blob: %w", err)
	}

	target := s.contentPath(descriptor.Digest)
	if err := os.Link(temporaryPath, target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("publish attachment blob: %w", err)
		}
		existing, availability, verifyErr := s.openVerified(descriptor)
		if existing != nil {
			verifyErr = errors.Join(verifyErr, existing.Close())
		}
		if verifyErr != nil {
			return verifyErr
		}
		if availability != domainattachment.BlobAvailabilityVerified {
			return fmt.Errorf(
				"existing attachment blob %s is %s",
				descriptor.Digest,
				availability,
			)
		}
	}
	if err := s.syncDirectory(s.contentDir()); err != nil {
		return fmt.Errorf("sync attachment content directory: %w", err)
	}
	return nil
}

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
