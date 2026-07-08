package attachment

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"mime"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.abhg.dev/cardamom/internal/configuration"
)

const (
	// MaxFilenameBytes is the longest portable attachment filename.
	MaxFilenameBytes = 255

	// MaxAttachmentSizeBytes is the built-in attachment admission default.
	MaxAttachmentSizeBytes uint64 = configuration.DefaultAttachmentMaxBytes
)

// Digest is the canonical SHA-256 identity of immutable blob content.
type Digest string

// NewDigest parses sha256 followed by 64 lowercase hexadecimal digits.
func NewDigest(value string) (Digest, error) {
	const prefix = "sha256:"
	encoded := strings.TrimPrefix(value, prefix)
	if len(encoded) != 64 || value != prefix+encoded || encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("invalid blob digest %q", value)
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		return "", fmt.Errorf("invalid blob digest %q", value)
	}
	return Digest(value), nil
}

// String returns the canonical SHA-256 descriptor digest.
func (d Digest) String() string { return string(d) }

// BlobDescriptor identifies immutable content by SHA-256 digest and byte size.
type BlobDescriptor struct {
	// Digest is the canonical SHA-256 content identity.
	Digest Digest // required

	// SizeBytes is the complete content size in bytes.
	SizeBytes uint64 // required
}

// Validate verifies that the descriptor identifies content accepted by v1.
func (d *BlobDescriptor) Validate() error {
	digest, err := NewDigest(d.Digest.String())
	if err != nil || digest != d.Digest {
		return errors.New("invalid blob descriptor: digest required")
	}
	if d.SizeBytes > math.MaxInt64 {
		return fmt.Errorf(
			"invalid blob descriptor: size %d exceeds representable maximum %d",
			d.SizeBytes,
			uint64(math.MaxInt64),
		)
	}
	return nil
}

// Filename is one validated portable path component used for presentation.
// It is metadata only and never selects blob storage paths.
type Filename string

// NewFilename parses a portable attachment filename.
func NewFilename(value string) (Filename, error) {
	if value == "" {
		return "", errors.New("attachment filename required")
	}
	if !utf8.ValidString(value) {
		return "", errors.New("attachment filename must be valid UTF-8")
	}
	if len(value) > MaxFilenameBytes {
		return "", fmt.Errorf(
			"attachment filename exceeds %d bytes",
			MaxFilenameBytes,
		)
	}
	if value == "." || value == ".." ||
		strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return "", fmt.Errorf("attachment filename %q is not portable", value)
	}
	for _, character := range value {
		if unicode.IsControl(character) || strings.ContainsRune(`<>:"/\|?*`, character) {
			return "", fmt.Errorf("attachment filename %q is not portable", value)
		}
	}

	base, _, _ := strings.Cut(value, ".")
	if windowsDeviceName(strings.ToUpper(base)) {
		return "", fmt.Errorf("attachment filename %q is not portable", value)
	}
	return Filename(value), nil
}

func windowsDeviceName(value string) bool {
	switch value {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(value) == 4 &&
		(value[:3] == "COM" || value[:3] == "LPT") &&
		value[3] >= '1' && value[3] <= '9'
}

// String returns the portable presentation filename.
func (f Filename) String() string { return string(f) }

// MediaType is a canonical MIME media type detected from attachment content.
type MediaType string

// NewMediaType parses and canonicalizes a MIME media type.
func NewMediaType(value string) (MediaType, error) {
	base, parameters, err := mime.ParseMediaType(value)
	if err != nil || base == "" {
		return "", fmt.Errorf("invalid attachment media type %q", value)
	}
	canonical := mime.FormatMediaType(base, parameters)
	if canonical == "" {
		return "", fmt.Errorf("invalid attachment media type %q", value)
	}
	return MediaType(canonical), nil
}

// String returns the canonical MIME media type.
func (m MediaType) String() string { return string(m) }

// IsInlineMediaType reports whether browser presentation may render an
// attachment's detected media type inline.
func IsInlineMediaType(mediaType MediaType) bool {
	switch mediaType.String() {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// BlobAvailability is one local observation of expected blob content.
// Availability is replica-local state and is independent of Lifecycle.
type BlobAvailability uint8

const (
	_ BlobAvailability = iota

	// BlobAvailabilityMissing means no local content exists for the descriptor.
	BlobAvailabilityMissing

	// BlobAvailabilityPresentUnverified means local content has the expected
	// size but has not been fully hashed by this observation.
	BlobAvailabilityPresentUnverified

	// BlobAvailabilitySizeMismatch means local content has an unexpected size.
	BlobAvailabilitySizeMismatch

	// BlobAvailabilityVerified means local content matches the descriptor.
	BlobAvailabilityVerified

	// BlobAvailabilityDigestMismatch means size-valid local content has an
	// unexpected SHA-256 digest.
	BlobAvailabilityDigestMismatch
)

var blobAvailabilityNames = [...]string{
	"",
	"missing",
	"present_unverified",
	"size_mismatch",
	"verified",
	"digest_mismatch",
}

// NewBlobAvailability parses a local blob availability state.
func NewBlobAvailability(value string) (BlobAvailability, error) {
	for availability := BlobAvailabilityMissing; availability <= BlobAvailabilityDigestMismatch; availability++ {
		if availability.String() == value {
			return availability, nil
		}
	}
	return 0, fmt.Errorf("invalid blob availability %q", value)
}

// String returns the stable local availability name.
func (a BlobAvailability) String() string {
	if int(a) >= len(blobAvailabilityNames) {
		return ""
	}
	return blobAvailabilityNames[a]
}
