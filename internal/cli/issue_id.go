package cli

import (
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
)

// issueID is a CLI issue reference normalized to its bare persisted identity.
// It accepts the same ID with or without the Markdown reference marker.
type issueID string

// UnmarshalText parses one command-line issue reference.
func (id *issueID) UnmarshalText(text []byte) error {
	parsed, err := parseIssueID(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

func parseIssueID(value string) (issueID, error) {
	parsed, err := issue.NewID(issueIDReference(value))
	if err != nil {
		return "", err
	}
	return issueID(parsed.String()), nil
}

func issueIDReference(value string) string {
	return strings.TrimPrefix(value, "%")
}

func (id issueID) String() string { return string(id) }

func issueIDStrings(ids []issueID) []string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}
	return values
}

// optionalIssueID adds the empty value used to clear an issue relationship.
type optionalIssueID issueID

// UnmarshalText parses an issue reference or the empty relationship value.
func (id *optionalIssueID) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*id = ""
		return nil
	}
	parsed, err := parseIssueID(string(text))
	if err != nil {
		return err
	}
	*id = optionalIssueID(parsed)
	return nil
}

func optionalIssueIDString(id *optionalIssueID) *string {
	if id == nil {
		return nil
	}
	value := string(*id)
	return &value
}
