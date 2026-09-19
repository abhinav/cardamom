package boardhistory

import (
	"errors"
	"strings"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssueLabel assigns or removes one label.
type IssueLabel struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Label is the nonblank label spelling retained for this assignment or removal.
	Label string // required

	// Present reports whether the label is assigned after this event.
	Present bool
}

// EventIssueLabel assigns or removes an issue label.
const EventIssueLabel EventKind = "issue.label"

func (*IssueLabel) kind() EventKind { return EventIssueLabel }

func (l *IssueLabel) validateV1() error {
	if err := requireIssue(l.IssueUID); err != nil {
		return err
	}
	if strings.TrimSpace(l.Label) == "" {
		return errors.New("label event requires a label")
	}
	return nil
}

func (l *IssueLabel) valueV1() any {
	return issueLabelV1{IssueUID: l.IssueUID.Bytes(), Label: l.Label, Present: l.Present}
}

type issueLabelV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Label    string `json:"label"`
	Present  bool   `json:"present"`
}

func decodeIssueLabelV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueLabelV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssueLabel{IssueUID: uid, Label: value.Label, Present: value.Present}, nil
}

// IssueDependency assigns or removes one prerequisite edge.
type IssueDependency struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// PrerequisiteUID identifies the issue required by IssueUID; the two identities must differ.
	PrerequisiteUID issue.UID // required

	// Present reports whether the dependency exists after this event.
	Present bool
}

// EventIssueDependency assigns or removes an issue dependency.
const EventIssueDependency EventKind = "issue.dependency"

func (*IssueDependency) kind() EventKind { return EventIssueDependency }

func (d *IssueDependency) validateV1() error {
	return requireDistinctIssues(d.IssueUID, d.PrerequisiteUID)
}

func (d *IssueDependency) valueV1() any {
	return issueDependencyV1{
		IssueUID:        d.IssueUID.Bytes(),
		PrerequisiteUID: d.PrerequisiteUID.Bytes(),
		Present:         d.Present,
	}
}

type issueDependencyV1 struct {
	IssueUID        []byte `json:"issue_uid"`
	PrerequisiteUID []byte `json:"prerequisite_uid"`
	Present         bool   `json:"present"`
}

func decodeIssueDependencyV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueDependencyV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	prerequisite, err := issue.ParseUID(value.PrerequisiteUID)
	if err != nil {
		return nil, err
	}
	return &IssueDependency{
		IssueUID: uid, PrerequisiteUID: prerequisite, Present: value.Present,
	}, nil
}

// IssueContainment assigns or removes one parent edge.
type IssueContainment struct {
	// ChildUID identifies the issue placed under or removed from ParentUID.
	ChildUID issue.UID // required

	// ParentUID identifies the containing issue and must differ from ChildUID.
	ParentUID issue.UID // required

	// Present reports whether the containment edge exists after this event.
	Present bool
}

// EventIssueContainment assigns or removes an issue parent.
const EventIssueContainment EventKind = "issue.containment"

func (*IssueContainment) kind() EventKind { return EventIssueContainment }

func (c *IssueContainment) validateV1() error {
	return requireDistinctIssues(c.ChildUID, c.ParentUID)
}

func (c *IssueContainment) valueV1() any {
	return issueContainmentV1{
		ChildUID: c.ChildUID.Bytes(), ParentUID: c.ParentUID.Bytes(), Present: c.Present,
	}
}

type issueContainmentV1 struct {
	ChildUID  []byte `json:"child_uid"`
	ParentUID []byte `json:"parent_uid"`
	Present   bool   `json:"present"`
}

func decodeIssueContainmentV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueContainmentV1](encoded)
	if err != nil {
		return nil, err
	}
	child, err := issue.ParseUID(value.ChildUID)
	if err != nil {
		return nil, err
	}
	parent, err := issue.ParseUID(value.ParentUID)
	if err != nil {
		return nil, err
	}
	return &IssueContainment{
		ChildUID: child, ParentUID: parent, Present: value.Present,
	}, nil
}

// ExternalKey assigns or removes one board-scoped external key.
type ExternalKey struct {
	// Key is a nonempty board-scoped key; whitespace is retained without trimming.
	Key string // required

	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Present reports whether the key resolves to IssueUID after this event.
	Present bool
}

// EventExternalKey assigns or removes a board-scoped external key.
const EventExternalKey EventKind = "board.external_key"

func (*ExternalKey) kind() EventKind { return EventExternalKey }

func (k *ExternalKey) validateV1() error {
	if err := requireIssue(k.IssueUID); err != nil {
		return err
	}
	if k.Key == "" {
		return errors.New("external key event requires a key")
	}
	return nil
}

func (k *ExternalKey) valueV1() any {
	return externalKeyV1{Key: k.Key, IssueUID: k.IssueUID.Bytes(), Present: k.Present}
}

type externalKeyV1 struct {
	Key      string `json:"key"`
	IssueUID []byte `json:"issue_uid"`
	Present  bool   `json:"present"`
}

func decodeExternalKeyV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[externalKeyV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &ExternalKey{Key: value.Key, IssueUID: uid, Present: value.Present}, nil
}

// BoardPin assigns one pin position or removes the pin.
type BoardPin struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Present reports whether the issue is pinned after this event.
	Present bool

	// Position is positive when Present is true and zero otherwise.
	Position int64
}

// EventBoardPin assigns or removes an issue pin.
const EventBoardPin EventKind = "board.pin"

func (*BoardPin) kind() EventKind { return EventBoardPin }

func (p *BoardPin) validateV1() error {
	if err := requireIssue(p.IssueUID); err != nil {
		return err
	}
	if p.Present && p.Position <= 0 {
		return errors.New("present board pin requires a positive position")
	}
	if !p.Present && p.Position != 0 {
		return errors.New("removed board pin must not contain a position")
	}
	return nil
}

func (p *BoardPin) valueV1() any {
	return boardPinV1{
		IssueUID: p.IssueUID.Bytes(), Present: p.Present, Position: p.Position,
	}
}

type boardPinV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Present  bool   `json:"present"`
	Position int64  `json:"position"`
}

func decodeBoardPinV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[boardPinV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &BoardPin{
		IssueUID: uid, Present: value.Present, Position: value.Position,
	}, nil
}

func requireDistinctIssues(first, second issue.UID) error {
	if err := requireIssue(first); err != nil {
		return err
	}
	if err := requireIssue(second); err != nil {
		return err
	}
	if first == second {
		return errors.New("relationship endpoints must differ")
	}
	return nil
}
