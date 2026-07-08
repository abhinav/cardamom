package dump

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

// preflightAttachmentFiles verifies access to each immutable content handle
// without retaining handles across attachments or destination inspection.
func preflightAttachmentFiles(files []*GeneratedFile) error {
	if err := validateAttachmentFilePairs(files); err != nil {
		return err
	}
	for _, file := range files {
		if file.attachmentRole != attachmentGeneratedContent {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		if err := reader.Close(); err != nil {
			return fmt.Errorf("close preflight attachment %q: %w", file.Path(), err)
		}
	}
	return nil
}

func validateAttachmentGeneratedFile(file *GeneratedFile) error {
	if file.attachment == nil {
		return fmt.Errorf("generated attachment path %q has no sidecar metadata", file.Path())
	}
	if err := file.attachment.validate(); err != nil {
		return fmt.Errorf("generated attachment path %q: %w", file.Path(), err)
	}
	id := file.attachment.AttachmentID
	var expectedPath, expectedIdentity string
	switch file.attachmentRole {
	case attachmentGeneratedSidecar:
		expectedPath = path.Join("attachments", id, "metadata.yaml")
		expectedIdentity = "attachment:" + id + ":metadata"
	case attachmentGeneratedContent:
		expectedPath = path.Join("attachments", id, "files", file.attachment.Filename)
		expectedIdentity = "attachment:" + id + ":content"
	default:
		return fmt.Errorf("generated attachment path %q has invalid role", file.Path())
	}
	if file.Path() != expectedPath {
		return fmt.Errorf("generated attachment path %q requires path %q", file.Path(), expectedPath)
	}
	if file.Identity() != expectedIdentity {
		return fmt.Errorf("generated path %q requires identity %q", file.Path(), expectedIdentity)
	}
	return nil
}

func validateAttachmentFilePairs(files []*GeneratedFile) error {
	type roles struct {
		sidecar  bool
		content  bool
		metadata *attachmentSidecar
	}
	byID := make(map[string]roles)
	for _, file := range files {
		if err := validateGeneratedFile(file); err != nil {
			return err
		}
		if file.attachmentRole == attachmentGeneratedNone {
			continue
		}
		value := byID[file.attachment.AttachmentID]
		if value.metadata != nil && *value.metadata != *file.attachment {
			return fmt.Errorf("attachment %q generated files have different metadata", file.attachment.AttachmentID)
		}
		value.metadata = file.attachment
		switch file.attachmentRole {
		case attachmentGeneratedSidecar:
			if value.sidecar {
				return fmt.Errorf("attachment %q has duplicate sidecars", file.attachment.AttachmentID)
			}
			value.sidecar = true
		case attachmentGeneratedContent:
			if value.content {
				return fmt.Errorf("attachment %q has duplicate content", file.attachment.AttachmentID)
			}
			value.content = true
		}
		byID[file.attachment.AttachmentID] = value
	}
	for id, value := range byID {
		if !value.sidecar || !value.content {
			return fmt.Errorf("attachment %q requires one sidecar and one content file", id)
		}
	}
	return nil
}

func authorizeAttachmentPath(
	publication Publication,
	generated *GeneratedFile,
	files fileSystem,
) error {
	metadataPath := attachmentSidecarPathFromMetadata(*generated.attachment)
	reader, err := files.Open(filepath.Join(
		publication.Destination,
		filepath.FromSlash(metadataPath),
	))
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("generated path %q collides with an unowned file", generated.Path())
	}
	if err != nil {
		return fmt.Errorf("open attachment sidecar %q: %w", metadataPath, err)
	}
	metadata, encoded, readErr := decodeAttachmentSidecar(reader)
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return fmt.Errorf("generated path %q has invalid ownership metadata: %w", generated.Path(), err)
	}
	if metadata != *generated.attachment {
		return modifiedAttachmentPath(publication, generated.Path())
	}

	switch generated.attachmentRole {
	case attachmentGeneratedSidecar:
		expected, err := yaml.Marshal(*generated.attachment)
		if err != nil {
			return fmt.Errorf("encode expected attachment sidecar: %w", err)
		}
		if !bytes.Equal(encoded, expected) {
			return modifiedAttachmentPath(publication, generated.Path())
		}
	case attachmentGeneratedContent:
		digest, size, err := fileDigest(filepath.Join(
			publication.Destination,
			filepath.FromSlash(generated.Path()),
		), files)
		if err != nil {
			return err
		}
		actualDigest := "sha256:" + hex.EncodeToString(digest[:])
		if size < 0 || uint64(size) != metadata.SizeBytes || actualDigest != metadata.Digest {
			return modifiedAttachmentPath(publication, generated.Path())
		}
	}
	return nil
}

func modifiedAttachmentPath(publication Publication, relative string) error {
	if publication.Force == ForceGenerated {
		return nil
	}
	return &GeneratedFileError{Path: relative, Kind: GeneratedFileModified}
}

func attachmentSidecarPathFromMetadata(metadata attachmentSidecar) string {
	return path.Join("attachments", metadata.AttachmentID, "metadata.yaml")
}

func stageAttachmentGeneratedFile(
	generated *GeneratedFile,
	destination string,
	files fileSystem,
) error {
	reader, err := generated.Open()
	if err != nil {
		return err
	}
	writer, err := files.Create(destination, 0o644)
	if err != nil {
		return errors.Join(fmt.Errorf("stage %q: %w", generated.Path(), err), reader.Close())
	}
	hash := sha256.New()
	written, readErr := io.Copy(io.MultiWriter(writer, hash), reader)
	closeErr := errors.Join(reader.Close(), writer.Close())
	if err := errors.Join(readErr, closeErr); err != nil {
		return fmt.Errorf("stage generated path %q: %w", generated.Path(), err)
	}
	if written != generated.Size() {
		return fmt.Errorf("generated path %q provided %d bytes, expected %d",
			generated.Path(), written, generated.Size())
	}
	actualDigest := hex.EncodeToString(hash.Sum(nil))
	var expectedDigest string
	switch generated.attachmentRole {
	case attachmentGeneratedSidecar:
		expected, err := yaml.Marshal(*generated.attachment)
		if err != nil {
			return fmt.Errorf("encode expected attachment sidecar: %w", err)
		}
		expectedDigest = digest(expected)
	case attachmentGeneratedContent:
		expectedDigest = strings.TrimPrefix(generated.attachment.Digest, "sha256:")
	}
	if actualDigest != expectedDigest {
		return fmt.Errorf("generated attachment path %q digest does not match sidecar", generated.Path())
	}
	return nil
}
