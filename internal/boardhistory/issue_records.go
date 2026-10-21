package boardhistory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssueLog records one immutable Log entry.
type IssueLog struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// ID is log_ or the retained legacy cmt_ prefix followed by
	// 32 lowercase hexadecimal digits.
	ID string // required

	// Kind is post or state_snapshot.
	Kind string // required

	// Body is the nonempty text of the immutable entry.
	Body string // required

	// Author names the actor represented by the entry, when recorded.
	Author *string

	// Committer names the actor that stored the entry, when recorded.
	Committer *string

	// CreatedAt records the entry time, when recorded.
	CreatedAt *time.Time

	// NextAction records the planned next action, when present.
	NextAction *string
}

// EventIssueLog records one immutable Log entry.
const EventIssueLog EventKind = "issue.log"

func (*IssueLog) kind() EventKind { return EventIssueLog }

func (l *IssueLog) validateV1() error {
	if err := requireIssue(l.IssueUID); err != nil {
		return err
	}
	if err := validateLogIDV1(l.ID); err != nil {
		return err
	}
	if err := validateLogKindV1(l.Kind); err != nil {
		return err
	}
	if l.Body == "" {
		return errors.New("issue Log requires a body")
	}
	return nil
}

func (l *IssueLog) valueV1() any {
	return issueLogV1{
		IssueUID:   l.IssueUID.Bytes(),
		ID:         l.ID,
		Kind:       l.Kind,
		Author:     l.Author,
		Committer:  l.Committer,
		Body:       l.Body,
		CreatedAt:  timePointerUTC(l.CreatedAt),
		NextAction: l.NextAction,
	}
}

type issueLogV1 struct {
	IssueUID   []byte     `json:"issue_uid"`
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Author     *string    `json:"author"`
	Committer  *string    `json:"committer"`
	Body       string     `json:"body"`
	CreatedAt  *time.Time `json:"created_at"`
	NextAction *string    `json:"next_action"`
}

func decodeIssueLogV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueLogV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssueLog{
		IssueUID: uid, ID: value.ID, Kind: value.Kind,
		Author: value.Author, Committer: value.Committer, Body: value.Body,
		CreatedAt: value.CreatedAt, NextAction: value.NextAction,
	}, nil
}

// IssueState assigns or clears the current replaceable recovery record.
type IssueState struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// State is nil to clear the recovery record.
	State *StateRecord
}

// EventIssueState assigns or clears the issue State record.
const EventIssueState EventKind = "issue.state"

// StateRecord is the complete retained recovery record assigned by IssueState.
type StateRecord struct {
	// Body is the nonblank text of the current recovery record.
	Body string // required

	// Author identifies the actor that updated the record.
	// It must be present if and only if UpdatedAt is present.
	Author *string

	// UpdatedAt records when Author updated the record.
	// It must be present if and only if Author is present.
	UpdatedAt *time.Time

	// SnapshotLogEntryID identifies the Log entry represented by this record.
	// When present, it follows the retained spelling rules of [IssueLog.ID].
	SnapshotLogEntryID *string

	// NextAction records the planned next action, when present.
	NextAction *string
}

func (*IssueState) kind() EventKind { return EventIssueState }

func (s *IssueState) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	if s.State == nil {
		return nil
	}
	if strings.TrimSpace(s.State.Body) == "" ||
		(s.State.Author == nil) != (s.State.UpdatedAt == nil) {
		return errors.New("present State requires a body and paired attribution")
	}
	if s.State.SnapshotLogEntryID != nil {
		return validateLogIDV1(*s.State.SnapshotLogEntryID)
	}
	return nil
}

func (s *IssueState) valueV1() any {
	value := issueStateV1{IssueUID: s.IssueUID.Bytes()}
	if s.State == nil {
		return value
	}
	value.Present = true
	value.Body = s.State.Body
	value.Author = s.State.Author
	value.UpdatedAt = timePointerUTC(s.State.UpdatedAt)
	value.SnapshotLogEntryID = s.State.SnapshotLogEntryID
	value.NextAction = s.State.NextAction
	return value
}

type issueStateV1 struct {
	IssueUID           []byte     `json:"issue_uid"`
	Present            bool       `json:"present"`
	Body               string     `json:"body"`
	Author             *string    `json:"author"`
	UpdatedAt          *time.Time `json:"updated_at"`
	SnapshotLogEntryID *string    `json:"snapshot_log_entry_id"`
	NextAction         *string    `json:"next_action"`
}

func decodeIssueStateV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueStateV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	if !value.Present {
		if value.Body != "" || value.Author != nil || value.UpdatedAt != nil ||
			value.SnapshotLogEntryID != nil || value.NextAction != nil {
			return nil, errors.New("cleared State must not contain record fields")
		}
		return &IssueState{IssueUID: uid}, nil
	}
	return &IssueState{
		IssueUID: uid,
		State: &StateRecord{
			Body: value.Body, Author: value.Author, UpdatedAt: value.UpdatedAt,
			SnapshotLogEntryID: value.SnapshotLogEntryID, NextAction: value.NextAction,
		},
	}, nil
}

// IssueResult assigns or clears the current durable outcome.
type IssueResult struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Body is nil to clear the durable outcome, or contains nonblank text.
	Body *string
}

// EventIssueResult assigns or clears the issue Result.
const EventIssueResult EventKind = "issue.result"

func (*IssueResult) kind() EventKind { return EventIssueResult }

func (r *IssueResult) validateV1() error {
	if err := requireIssue(r.IssueUID); err != nil {
		return err
	}
	if r.Body != nil && strings.TrimSpace(*r.Body) == "" {
		return errors.New("present Result requires a body")
	}
	return nil
}

func (r *IssueResult) valueV1() any {
	if r.Body == nil {
		return issueResultV1{IssueUID: r.IssueUID.Bytes()}
	}
	return issueResultV1{IssueUID: r.IssueUID.Bytes(), Present: true, Body: *r.Body}
}

type issueResultV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Present  bool   `json:"present"`
	Body     string `json:"body"`
}

func decodeIssueResultV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueResultV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	if !value.Present {
		if value.Body != "" {
			return nil, errors.New("cleared Result must not contain a body")
		}
		return &IssueResult{IssueUID: uid}, nil
	}
	return &IssueResult{IssueUID: uid, Body: new(value.Body)}, nil
}

// IssueCheckpoint records one immutable checkpoint decision.
type IssueCheckpoint struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Outcome is approved or denied.
	Outcome string // required

	// DecidedAt is the nonzero time of the checkpoint decision.
	DecidedAt time.Time // required

	// Reason explains the checkpoint decision and may be empty.
	Reason string
}

// EventIssueCheckpoint records one checkpoint decision.
const EventIssueCheckpoint EventKind = "issue.checkpoint"

func (*IssueCheckpoint) kind() EventKind { return EventIssueCheckpoint }

func (c *IssueCheckpoint) validateV1() error {
	if err := requireIssue(c.IssueUID); err != nil {
		return err
	}
	if err := validateCheckpointOutcomeV1(c.Outcome); err != nil {
		return err
	}
	if c.DecidedAt.IsZero() {
		return errors.New("checkpoint requires a decision time")
	}
	return nil
}

func (c *IssueCheckpoint) valueV1() any {
	return issueCheckpointV1{
		IssueUID:  c.IssueUID.Bytes(),
		Outcome:   c.Outcome,
		Reason:    c.Reason,
		DecidedAt: c.DecidedAt.UTC(),
	}
}

type issueCheckpointV1 struct {
	IssueUID  []byte    `json:"issue_uid"`
	Outcome   string    `json:"outcome"`
	Reason    string    `json:"reason"`
	DecidedAt time.Time `json:"decided_at"`
}

func decodeIssueCheckpointV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueCheckpointV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssueCheckpoint{
		IssueUID: uid, Outcome: value.Outcome,
		Reason: value.Reason, DecidedAt: value.DecidedAt,
	}, nil
}

func validateLogIDV1(value string) error {
	body, ok := strings.CutPrefix(value, "log_")
	if !ok {
		body, ok = strings.CutPrefix(value, "cmt_")
	}
	if !ok || len(body) != 32 || body != strings.ToLower(body) {
		return fmt.Errorf("invalid V1 Log ID %q", value)
	}
	for _, character := range body {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return fmt.Errorf("invalid V1 Log ID %q", value)
		}
	}
	return nil
}

func validateLogKindV1(value string) error {
	if value != "post" && value != "state_snapshot" {
		return fmt.Errorf("invalid V1 Log kind %q", value)
	}
	return nil
}

func validateCheckpointOutcomeV1(value string) error {
	if value != "approved" && value != "denied" {
		return fmt.Errorf("invalid V1 checkpoint outcome %q", value)
	}
	return nil
}
