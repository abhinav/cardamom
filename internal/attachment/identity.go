package attachment

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

const (
	idEntropyBytes     = 16
	attachmentIDPrefix = "att_"
)

var attachmentIDEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// ID is a durable board-scoped attachment handle.
// Its text is att_ followed by the canonical lowercase base32 encoding of
// 128 bits of entropy.
type ID string

// NewID parses a canonical attachment handle.
func NewID(value string) (ID, error) {
	encoded := strings.TrimPrefix(value, attachmentIDPrefix)
	if len(encoded) != 26 ||
		value != attachmentIDPrefix+encoded ||
		encoded != strings.ToLower(encoded) {
		return "", fmt.Errorf("invalid attachment ID %q", value)
	}

	decoded, err := attachmentIDEncoding.DecodeString(strings.ToUpper(encoded))
	if err != nil || len(decoded) != idEntropyBytes {
		return "", fmt.Errorf("invalid attachment ID %q", value)
	}
	canonical := strings.ToLower(attachmentIDEncoding.EncodeToString(decoded))
	if encoded != canonical {
		return "", fmt.Errorf("invalid attachment ID %q", value)
	}
	return ID(value), nil
}

// NewRandomID generates an attachment handle from cryptographic entropy.
func NewRandomID() (ID, error) {
	return newID(rand.Reader)
}

func newID(entropy io.Reader) (ID, error) {
	value := make([]byte, idEntropyBytes)
	if _, err := io.ReadFull(entropy, value); err != nil {
		return "", fmt.Errorf("read attachment ID entropy: %w", err)
	}
	encoded := strings.ToLower(attachmentIDEncoding.EncodeToString(value))
	return ID(attachmentIDPrefix + encoded), nil
}

// String returns the canonical attachment handle.
func (id ID) String() string { return string(id) }

// Association identifies the attachment's board and optional originating
// issue. The issue association is organizational, not an access boundary.
type Association struct {
	boardID       board.ID
	originIssueID issue.ID
}

// NewBoardAssociation constructs an attachment association without an
// originating issue.
func NewBoardAssociation(boardID board.ID) (Association, error) {
	if _, err := board.NewID(boardID.String()); err != nil {
		return Association{}, errors.New("attachment board ID required")
	}
	return Association{boardID: boardID}, nil
}

// NewIssueAssociation constructs an attachment association for the issue that
// introduced it to the board.
func NewIssueAssociation(boardID board.ID, issueID issue.ID) (Association, error) {
	association, err := NewBoardAssociation(boardID)
	if err != nil {
		return Association{}, err
	}
	if _, err := issue.NewID(issueID.String()); err != nil {
		return Association{}, errors.New("attachment origin issue ID required")
	}
	association.originIssueID = issueID
	return association, nil
}

// BoardID returns the board that owns the attachment metadata.
func (a *Association) BoardID() board.ID { return a.boardID }

// OriginIssueID returns the issue that introduced the attachment when one is
// associated.
func (a *Association) OriginIssueID() (issue.ID, bool) {
	return a.originIssueID, a.originIssueID != ""
}

// Validate verifies the board and optional originating issue identity.
func (a *Association) Validate() error {
	if _, err := NewBoardAssociation(a.boardID); err != nil {
		return err
	}
	if a.originIssueID != "" {
		if _, err := issue.NewID(a.originIssueID.String()); err != nil {
			return errors.New("attachment origin issue ID required")
		}
	}
	return nil
}
