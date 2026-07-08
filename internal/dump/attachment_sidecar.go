package dump

import (
	"errors"
	"fmt"
	"io"

	"github.com/goccy/go-yaml"
	"go.abhg.dev/cardamom/internal/attachment"
)

const attachmentSidecarVersion = 1

// attachmentSidecar is the versioned ownership record for one attachment file.
type attachmentSidecar struct {
	// FormatVersion identifies the sidecar schema.
	FormatVersion int `yaml:"format_version"`

	// AttachmentID is the board-scoped attachment identity.
	AttachmentID string `yaml:"attachment_id"`

	// Digest is the canonical SHA-256 identity of Files content.
	Digest string `yaml:"digest"`

	// SizeBytes is the complete Files content size in bytes.
	SizeBytes uint64 `yaml:"size_bytes"`

	// MediaType is the server-detected canonical MIME media type.
	MediaType string `yaml:"media_type"`

	// Filename is the portable name of the adjacent Files content.
	Filename string `yaml:"filename"`
}

func newAttachmentSidecar(value attachment.Attachment) attachmentSidecar {
	return attachmentSidecar{
		FormatVersion: attachmentSidecarVersion,
		AttachmentID:  value.ID.String(),
		Digest:        value.Blob.Digest.String(),
		SizeBytes:     value.Blob.SizeBytes,
		MediaType:     value.MediaType.String(),
		Filename:      value.Filename.String(),
	}
}

func (m *attachmentSidecar) validate() error {
	if m.FormatVersion != attachmentSidecarVersion {
		return fmt.Errorf("unsupported attachment sidecar format version %d", m.FormatVersion)
	}
	id, err := attachment.NewID(m.AttachmentID)
	if err != nil {
		return errors.New("attachment sidecar ID is invalid")
	}
	digest, err := attachment.NewDigest(m.Digest)
	if err != nil {
		return errors.New("attachment sidecar digest is invalid")
	}
	descriptor := attachment.BlobDescriptor{Digest: digest, SizeBytes: m.SizeBytes}
	if err := descriptor.Validate(); err != nil {
		return fmt.Errorf("attachment sidecar descriptor: %w", err)
	}
	filename, err := attachment.NewFilename(m.Filename)
	if err != nil {
		return fmt.Errorf("attachment sidecar filename: %w", err)
	}
	mediaType, err := attachment.NewMediaType(m.MediaType)
	if err != nil {
		return fmt.Errorf("attachment sidecar media type: %w", err)
	}
	if id.String() != m.AttachmentID || digest.String() != m.Digest ||
		filename.String() != m.Filename || mediaType.String() != m.MediaType {
		return errors.New("attachment sidecar metadata is not canonical")
	}
	return nil
}

func decodeAttachmentSidecar(reader io.Reader) (attachmentSidecar, []byte, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return attachmentSidecar{}, nil, fmt.Errorf("read attachment sidecar: %w", err)
	}
	var metadata attachmentSidecar
	if err := yaml.Unmarshal(body, &metadata); err != nil {
		return attachmentSidecar{}, nil, fmt.Errorf("decode attachment sidecar: %w", err)
	}
	if err := metadata.validate(); err != nil {
		return attachmentSidecar{}, nil, err
	}
	return metadata, body, nil
}

// attachmentGeneratedRole identifies how a generated attachment file proves
// ownership and content integrity during publication.
type attachmentGeneratedRole uint8

const (
	attachmentGeneratedNone attachmentGeneratedRole = iota
	attachmentGeneratedSidecar
	attachmentGeneratedContent
)
