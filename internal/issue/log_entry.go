package issue

import "fmt"

// LogEntryKind identifies one finite immutable Log payload.
type LogEntryKind int

const (
	logEntryKindUnknown LogEntryKind = iota

	// LogEntryKindPost is actor-authored immutable Markdown.
	LogEntryKindPost

	// LogEntryKindStateSnapshot is State preserved by an explicit or lifecycle commit.
	LogEntryKindStateSnapshot
)

// NewLogEntryKind parses one persisted Log entry kind.
func NewLogEntryKind(value string) (LogEntryKind, error) {
	switch value {
	case LogEntryKindPost.String():
		return LogEntryKindPost, nil
	case LogEntryKindStateSnapshot.String():
		return LogEntryKindStateSnapshot, nil
	default:
		return logEntryKindUnknown, fmt.Errorf("invalid log entry kind %q", value)
	}
}

// String returns the persisted Log entry kind.
func (k LogEntryKind) String() string {
	switch k {
	case LogEntryKindPost:
		return "post"
	case LogEntryKindStateSnapshot:
		return "state_snapshot"
	default:
		return ""
	}
}
