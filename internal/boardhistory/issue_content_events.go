package boardhistory

import (
	"errors"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssueSummarySet assigns or clears the issue summary.
type IssueSummarySet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Summary is nil to clear the issue summary, or contains nonblank text.
	Summary *string
}

// EventIssueSummarySet assigns or clears an issue summary.
const EventIssueSummarySet EventKind = "issue.summary.set"

func (*IssueSummarySet) kind() EventKind { return EventIssueSummarySet }

func (s *IssueSummarySet) validateV1() error {
	return validateOptionalIssueText(s.IssueUID, s.Summary, "issue summary")
}

func (s *IssueSummarySet) valueV1() any {
	return issueSummarySetV1{
		IssueUID: s.IssueUID.Bytes(),
		Summary:  optionalTextToV1(s.Summary),
	}
}

type issueSummarySetV1 struct {
	IssueUID []byte         `json:"issue_uid"`
	Summary  optionalTextV1 `json:"summary"`
}

func decodeIssueSummarySetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueSummarySetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	summary, err := value.Summary.toText()
	if err != nil {
		return nil, err
	}
	return &IssueSummarySet{IssueUID: uid, Summary: summary}, nil
}

// IssueDetailsSet assigns or clears the issue details.
type IssueDetailsSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Details is nil to clear the issue details, or contains nonblank text.
	Details *string
}

// EventIssueDetailsSet assigns or clears issue details.
const EventIssueDetailsSet EventKind = "issue.details.set"

func (*IssueDetailsSet) kind() EventKind { return EventIssueDetailsSet }

func (s *IssueDetailsSet) validateV1() error {
	return validateOptionalIssueText(s.IssueUID, s.Details, "issue details")
}

func (s *IssueDetailsSet) valueV1() any {
	return issueDetailsSetV1{
		IssueUID: s.IssueUID.Bytes(),
		Details:  optionalTextToV1(s.Details),
	}
}

type issueDetailsSetV1 struct {
	IssueUID []byte         `json:"issue_uid"`
	Details  optionalTextV1 `json:"details"`
}

func decodeIssueDetailsSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueDetailsSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	details, err := value.Details.toText()
	if err != nil {
		return nil, err
	}
	return &IssueDetailsSet{IssueUID: uid, Details: details}, nil
}

func validateOptionalIssueText(uid issue.UID, value *string, name string) error {
	if err := requireIssue(uid); err != nil {
		return err
	}
	if value != nil && strings.TrimSpace(*value) == "" {
		return errors.New(name + " assignment must not be blank")
	}
	return nil
}
