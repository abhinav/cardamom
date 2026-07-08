package dump

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"go.abhg.dev/cardamom/internal/markdown"
)

const ownershipVersion = 1

var errNotOwned = errors.New("file has no Cardamom dump ownership metadata")

// ownershipFrontmatter reserves the `cardamom` mapping for generated-file metadata.
type ownershipFrontmatter struct {
	// Cardamom is nil when authored frontmatter does not claim Cardamom ownership.
	Cardamom *ownershipMetadata `yaml:"cardamom"`
}

// ownershipMetadata identifies one generated body and its source independently
// of the canonical collection path.
type ownershipMetadata struct {
	// Version identifies the private ownership format.
	Version int `yaml:"format_version"`

	// ProjectID identifies the project represented by the generated file.
	ProjectID string `yaml:"project_id,omitempty"`

	// ProjectName names the project represented by the generated file.
	ProjectName string `yaml:"project_name,omitempty"`

	// BoardID prevents one board from replacing another board's generated file.
	BoardID string `yaml:"board_id"`

	// BoardName names the board represented by the generated file.
	BoardName string `yaml:"board_name,omitempty"`

	// Identity remains stable across canonical path changes.
	Identity string `yaml:"generated_identity"`

	// BodySHA256 covers the authored Markdown bytes after the frontmatter.
	BodySHA256 string `yaml:"body_sha256"`
}

func encodeOwnedFile(provenance Provenance, identity string, body []byte) ([]byte, error) {
	metadata := ownershipMetadata{
		Version:   ownershipVersion,
		ProjectID: provenance.ProjectID, ProjectName: provenance.ProjectName,
		BoardID: provenance.BoardID, BoardName: provenance.BoardName,
		Identity:   identity,
		BodySHA256: digest(body),
	}
	if err := validateOwnershipMetadata(metadata); err != nil {
		return nil, err
	}
	result, err := markdown.EncodeFrontmatter(ownershipFrontmatter{Cardamom: &metadata}, body)
	if err != nil {
		return nil, fmt.Errorf("encode ownership frontmatter: %w", err)
	}
	return result, nil
}

func decodeOwnedFile(file []byte) (ownershipMetadata, []byte, error) {
	var ownership ownershipFrontmatter
	body, found, err := markdown.DecodeFrontmatter(file, &ownership)
	if !found {
		return ownershipMetadata{}, nil, errNotOwned
	}
	if err != nil {
		return ownershipMetadata{}, nil, fmt.Errorf("decode ownership frontmatter: %w", err)
	}
	if ownership.Cardamom == nil {
		return ownershipMetadata{}, nil, errNotOwned
	}
	if err := validateOwnershipMetadata(*ownership.Cardamom); err != nil {
		return ownershipMetadata{}, nil, err
	}

	return *ownership.Cardamom, body, nil
}

// decodeOwnedReader validates ownership metadata and returns the digest of the
// authored body without retaining that body in memory.
func decodeOwnedReader(reader io.Reader) (ownershipMetadata, string, error) {
	var ownership ownershipFrontmatter
	body, found, err := markdown.DecodeFrontmatterReader(reader, &ownership)
	if err != nil {
		return ownershipMetadata{}, "", fmt.Errorf("decode ownership frontmatter: %w", err)
	}
	if !found {
		return ownershipMetadata{}, "", errNotOwned
	}
	if ownership.Cardamom == nil {
		return ownershipMetadata{}, "", errNotOwned
	}
	if err := validateOwnershipMetadata(*ownership.Cardamom); err != nil {
		return ownershipMetadata{}, "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, body); err != nil {
		return ownershipMetadata{}, "", fmt.Errorf("read generated body: %w", err)
	}
	return *ownership.Cardamom, hex.EncodeToString(hash.Sum(nil)), nil
}

func validateOwnershipMetadata(metadata ownershipMetadata) error {
	if metadata.Version != ownershipVersion {
		return fmt.Errorf("unsupported ownership format version %d", metadata.Version)
	}
	if metadata.BoardID == "" {
		return errors.New("ownership board ID is required")
	}
	if metadata.Identity == "" {
		return errors.New("generated identity is required")
	}
	if len(metadata.BodySHA256) != sha256.Size*2 {
		return errors.New("ownership body digest is invalid")
	}
	if _, err := hex.DecodeString(metadata.BodySHA256); err != nil {
		return errors.New("ownership body digest is invalid")
	}
	return nil
}
