package boardhistory

import (
	"errors"
	"fmt"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssuePublicIDSet assigns the issue's public ID.
type IssuePublicIDSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// PublicID follows the retained spelling rules of IssueOrigin.PublicID.
	PublicID string // required
}

// EventIssuePublicIDSet assigns an issue's public ID.
const EventIssuePublicIDSet EventKind = "issue.public_id.set"

func (*IssuePublicIDSet) kind() EventKind { return EventIssuePublicIDSet }

func (s *IssuePublicIDSet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	return validateIssueIDV1(s.PublicID)
}

func (s *IssuePublicIDSet) valueV1() any {
	return issuePublicIDSetV1{IssueUID: s.IssueUID.Bytes(), PublicID: s.PublicID}
}

type issuePublicIDSetV1 struct {
	IssueUID []byte `json:"issue_uid"`
	PublicID string `json:"public_id"`
}

func decodeIssuePublicIDSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issuePublicIDSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssuePublicIDSet{IssueUID: uid, PublicID: value.PublicID}, nil
}

// IssueTitleSet assigns the issue title.
type IssueTitleSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Title is the nonblank replacement issue title.
	Title string // required
}

// EventIssueTitleSet assigns an issue title.
const EventIssueTitleSet EventKind = "issue.title.set"

func (*IssueTitleSet) kind() EventKind { return EventIssueTitleSet }

func (s *IssueTitleSet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	if strings.TrimSpace(s.Title) == "" {
		return errors.New("issue title assignment requires a value")
	}
	return nil
}

func (s *IssueTitleSet) valueV1() any {
	return issueTitleSetV1{IssueUID: s.IssueUID.Bytes(), Title: s.Title}
}

type issueTitleSetV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Title    string `json:"title"`
}

func decodeIssueTitleSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueTitleSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssueTitleSet{IssueUID: uid, Title: value.Title}, nil
}

// IssueKindSet assigns the issue kind.
type IssueKindSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Kind is workstream, task, checkpoint, or routine.
	Kind string // required
}

// EventIssueKindSet assigns an issue kind.
const EventIssueKindSet EventKind = "issue.kind.set"

func (*IssueKindSet) kind() EventKind { return EventIssueKindSet }

func (s *IssueKindSet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	return validateIssueKindV1(s.Kind)
}

func (s *IssueKindSet) valueV1() any {
	return issueKindSetV1{IssueUID: s.IssueUID.Bytes(), Kind: s.Kind}
}

type issueKindSetV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Kind     string `json:"kind"`
}

func decodeIssueKindSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueKindSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssueKindSet{IssueUID: uid, Kind: value.Kind}, nil
}

func validateIssueIDV1(value string) error {
	for index, character := range []byte(value) {
		if isASCIILetter(character) || isASCIIDigit(character) || index > 0 && character == '-' {
			continue
		}
		return fmt.Errorf("invalid V1 issue ID %q", value)
	}
	if value == "" {
		return errors.New("V1 issue ID required")
	}
	return nil
}

func validateIssueKindV1(value string) error {
	switch value {
	case "workstream", "task", "checkpoint", "routine":
		return nil
	default:
		return fmt.Errorf("invalid V1 issue kind %q", value)
	}
}

func requireIssue(uid issue.UID) error {
	_, err := issue.ParseUID(uid.Bytes())
	return err
}
