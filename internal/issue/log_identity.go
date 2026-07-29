package issue

import (
	"fmt"
	"strings"
)

const logIDPrefix = "log_"

// LogID is a durable authored-log identity. New identities use log_ followed
// by 32 lowercase hexadecimal digits; historical cmt_ identities remain valid.
type LogID string

// NewLogID parses a current or historical log identity.
func NewLogID(value string) (LogID, error) {
	prefix := logIDPrefix
	body := strings.TrimPrefix(value, prefix)
	if body == value {
		prefix = "cmt_"
		body = strings.TrimPrefix(value, prefix)
	}
	if len(body) != 32 || value != prefix+body || body != strings.ToLower(body) {
		return "", fmt.Errorf("invalid log ID %q", value)
	}
	for _, character := range body {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return "", fmt.Errorf("invalid log ID %q", value)
		}
	}
	return LogID(value), nil
}

// String returns the durable log identity.
func (id LogID) String() string { return string(id) }

// LogReference identifies the issue that owns one current log entry.
type LogReference struct {
	// LogID identifies the referenced log entry.
	LogID LogID // required

	// IssueID identifies the issue that owns the log entry.
	IssueID ID // required
}
