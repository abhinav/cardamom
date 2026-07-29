package mail

import (
	"fmt"
	"strings"
)

const messageIDPrefix = "mail_"

// ID is a durable mailbox delivery identity.
// Its text is mail_ followed by 32 lowercase hexadecimal digits.
type ID string

// NewID parses a canonical mailbox delivery identity.
func NewID(value string) (ID, error) {
	body := strings.TrimPrefix(value, messageIDPrefix)
	if len(body) != 32 || value != messageIDPrefix+body || body != strings.ToLower(body) {
		return "", fmt.Errorf("invalid mail ID %q", value)
	}
	for _, character := range body {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", fmt.Errorf("invalid mail ID %q", value)
		}
	}
	return ID(value), nil
}

// String returns the canonical mailbox delivery identity.
func (id ID) String() string { return string(id) }
