package boardhistory

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssueLifecycleSet assigns the lifecycle and its associated closure time.
type IssueLifecycleSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Lifecycle is open, closed, or cancelled;
	// it determines whether ClosedAt is required.
	Lifecycle string // required

	// ClosedAt is nonzero when Lifecycle is closed or cancelled, and nil otherwise.
	ClosedAt *time.Time
}

// EventIssueLifecycleSet assigns an issue lifecycle and closure time.
const EventIssueLifecycleSet EventKind = "issue.lifecycle.set"

func (*IssueLifecycleSet) kind() EventKind { return EventIssueLifecycleSet }

func (s *IssueLifecycleSet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	return validateLifecycle(s.Lifecycle, s.ClosedAt)
}

func (s *IssueLifecycleSet) valueV1() any {
	return issueLifecycleSetV1{
		IssueUID:  s.IssueUID.Bytes(),
		Lifecycle: s.Lifecycle,
		ClosedAt:  optionalTimeToV1(s.ClosedAt),
	}
}

type issueLifecycleSetV1 struct {
	IssueUID  []byte         `json:"issue_uid"`
	Lifecycle string         `json:"lifecycle"`
	ClosedAt  optionalTimeV1 `json:"closed_at"`
}

func decodeIssueLifecycleSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueLifecycleSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	closedAt, err := value.ClosedAt.toTime()
	if err != nil {
		return nil, err
	}
	return &IssueLifecycleSet{IssueUID: uid, Lifecycle: value.Lifecycle, ClosedAt: closedAt}, nil
}

func validateLifecycle(value string, closedAt *time.Time) error {
	terminal, err := validateIssueLifecycleV1(value)
	if err != nil {
		return err
	}
	if terminal != (closedAt != nil) {
		return errors.New("issue lifecycle and closure time must be paired")
	}
	if closedAt != nil && closedAt.IsZero() {
		return errors.New("issue closure time must not be zero")
	}
	return nil
}

func validateIssueLifecycleV1(value string) (bool, error) {
	switch value {
	case "open":
		return false, nil
	case "closed", "cancelled":
		return true, nil
	default:
		return false, fmt.Errorf("invalid V1 issue lifecycle %q", value)
	}
}

// IssuePrioritySet assigns the issue priority.
type IssuePrioritySet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Priority is an integer from 0 through 4, inclusive.
	Priority int64 // required
}

// EventIssuePrioritySet assigns an issue priority.
const EventIssuePrioritySet EventKind = "issue.priority.set"

func (*IssuePrioritySet) kind() EventKind { return EventIssuePrioritySet }

func (s *IssuePrioritySet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	return validateIssuePriorityV1(s.Priority)
}

func (s *IssuePrioritySet) valueV1() any {
	return issuePrioritySetV1{IssueUID: s.IssueUID.Bytes(), Priority: s.Priority}
}

type issuePrioritySetV1 struct {
	IssueUID []byte `json:"issue_uid"`
	Priority int64  `json:"priority"`
}

func decodeIssuePrioritySetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issuePrioritySetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	return &IssuePrioritySet{IssueUID: uid, Priority: value.Priority}, nil
}

func validateIssuePriorityV1(value int64) error {
	if value < 0 || value > 4 {
		return fmt.Errorf("invalid V1 issue priority %d", value)
	}
	return nil
}

// IssueWaitingSet assigns the issue's complete waiting state.
type IssueWaitingSet struct {
	// IssueUID identifies the affected issue by its stable identity.
	IssueUID issue.UID // required

	// Waiting is nil to clear the waiting state.
	// A waiting state requires a nonblank reason and nonzero Since.
	Waiting *issue.WaitingState
}

// EventIssueWaitingSet assigns or clears an issue waiting state.
const EventIssueWaitingSet EventKind = "issue.waiting.set"

func (*IssueWaitingSet) kind() EventKind { return EventIssueWaitingSet }

func (s *IssueWaitingSet) validateV1() error {
	if err := requireIssue(s.IssueUID); err != nil {
		return err
	}
	return validateWaiting(s.Waiting)
}

func (s *IssueWaitingSet) valueV1() any {
	return issueWaitingSetV1{
		IssueUID: s.IssueUID.Bytes(),
		Waiting:  waitingToV1(s.Waiting),
	}
}

type issueWaitingSetV1 struct {
	IssueUID []byte    `json:"issue_uid"`
	Waiting  waitingV1 `json:"waiting"`
}

func decodeIssueWaitingSetV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueWaitingSetV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.IssueUID)
	if err != nil {
		return nil, err
	}
	waiting, err := value.Waiting.toWaitingState()
	if err != nil {
		return nil, err
	}
	return &IssueWaitingSet{IssueUID: uid, Waiting: waiting}, nil
}

type waitingV1 struct {
	Waiting bool      `json:"waiting"`
	Reason  string    `json:"reason"`
	Since   time.Time `json:"since"`
}

func waitingToV1(value *issue.WaitingState) waitingV1 {
	if value == nil {
		return waitingV1{}
	}
	return waitingV1{Waiting: true, Reason: value.Reason, Since: value.Since.UTC()}
}

func (v *waitingV1) toWaitingState() (*issue.WaitingState, error) {
	if !v.Waiting {
		if v.Reason != "" || !v.Since.IsZero() {
			return nil, errors.New("ready assignment must not contain waiting metadata")
		}
		return nil, nil
	}
	return &issue.WaitingState{Reason: v.Reason, Since: v.Since}, nil
}

func validateWaiting(value *issue.WaitingState) error {
	if value == nil {
		return nil
	}
	if strings.TrimSpace(value.Reason) == "" || value.Since.IsZero() {
		return errors.New("waiting assignment requires reason and time")
	}
	return nil
}
