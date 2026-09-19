package boardhistory

import (
	"errors"
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/issue"
)

// IssueOrigin records one issue's identity and complete scalar projection.
type IssueOrigin struct {
	// UID is the stable issue identity, independent of its public ID.
	UID issue.UID // required

	// PublicID starts with an ASCII letter or digit and contains only
	// ASCII letters, digits, and hyphens.
	PublicID string // required

	// Title is the nonblank stored issue title.
	Title string // required

	// Kind is workstream, task, checkpoint, or routine.
	Kind string // required

	// Lifecycle is open, closed, or cancelled;
	// it determines whether ClosedAt is required.
	Lifecycle string // required

	// Priority is an integer from 0 through 4, inclusive.
	Priority int64 // required

	// CreatedAt is the nonzero issue creation time.
	CreatedAt time.Time // required

	// UpdatedAt is the nonzero time of the last stored issue update.
	UpdatedAt time.Time // required

	// ClosedAt is nonzero when Lifecycle is closed or cancelled, and nil otherwise.
	ClosedAt *time.Time

	// Waiting is nil when the issue is not waiting.
	// A waiting state requires a nonblank reason and nonzero Since.
	Waiting *issue.WaitingState

	// Summary is nil when absent, or contains nonblank text.
	Summary *string

	// Details is nil when absent, or contains nonblank text.
	Details *string
}

// EventIssueOrigin records one issue's initial or baseline projection.
const EventIssueOrigin EventKind = "issue.origin"

func (*IssueOrigin) kind() EventKind { return EventIssueOrigin }

func (o *IssueOrigin) validateV1() error {
	if err := requireIssue(o.UID); err != nil {
		return err
	}
	if strings.TrimSpace(o.PublicID) == "" || strings.TrimSpace(o.Title) == "" ||
		o.CreatedAt.IsZero() || o.UpdatedAt.IsZero() {
		return errors.New("issue origin requires core fields and times")
	}
	if err := validateIssueIDV1(o.PublicID); err != nil {
		return err
	}
	if err := validateIssueKindV1(o.Kind); err != nil {
		return err
	}
	if err := validateIssuePriorityV1(o.Priority); err != nil {
		return err
	}
	if err := validateLifecycle(o.Lifecycle, o.ClosedAt); err != nil {
		return err
	}
	if err := validateWaiting(o.Waiting); err != nil {
		return err
	}
	if o.Summary != nil && strings.TrimSpace(*o.Summary) == "" {
		return errors.New("issue origin summary must not be blank")
	}
	if o.Details != nil && strings.TrimSpace(*o.Details) == "" {
		return errors.New("issue origin details must not be blank")
	}
	return nil
}

func (o *IssueOrigin) valueV1() any {
	var waitingReason *string
	var waitingSince *time.Time
	if o.Waiting != nil {
		waitingReason = new(o.Waiting.Reason)
		waitingSince = timePointerUTC(&o.Waiting.Since)
	}
	return issueOriginV1{
		UID:           o.UID.Bytes(),
		PublicID:      o.PublicID,
		Title:         o.Title,
		Kind:          o.Kind,
		Lifecycle:     o.Lifecycle,
		Priority:      o.Priority,
		CreatedAt:     o.CreatedAt.UTC(),
		UpdatedAt:     o.UpdatedAt.UTC(),
		ClosedAt:      timePointerUTC(o.ClosedAt),
		WaitingReason: waitingReason,
		WaitingSince:  waitingSince,
		Summary:       o.Summary,
		Details:       o.Details,
	}
}

type issueOriginV1 struct {
	UID           []byte     `json:"uid"`
	PublicID      string     `json:"public_id"`
	Title         string     `json:"title"`
	Kind          string     `json:"kind"`
	Lifecycle     string     `json:"lifecycle"`
	Priority      int64      `json:"priority"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ClosedAt      *time.Time `json:"closed_at"`
	WaitingReason *string    `json:"waiting_reason"`
	WaitingSince  *time.Time `json:"waiting_since"`
	Summary       *string    `json:"summary"`
	Details       *string    `json:"details"`
}

func decodeIssueOriginV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[issueOriginV1](encoded)
	if err != nil {
		return nil, err
	}
	uid, err := issue.ParseUID(value.UID)
	if err != nil {
		return nil, err
	}
	if (value.WaitingReason == nil) != (value.WaitingSince == nil) {
		return nil, errors.New("issue waiting reason and time must be paired")
	}
	var waiting *issue.WaitingState
	if value.WaitingReason != nil {
		waiting = &issue.WaitingState{
			Reason: *value.WaitingReason,
			Since:  *value.WaitingSince,
		}
	}
	return &IssueOrigin{
		UID:       uid,
		PublicID:  value.PublicID,
		Title:     value.Title,
		Kind:      value.Kind,
		Lifecycle: value.Lifecycle,
		Priority:  value.Priority,
		CreatedAt: value.CreatedAt,
		UpdatedAt: value.UpdatedAt,
		ClosedAt:  value.ClosedAt,
		Waiting:   waiting,
		Summary:   value.Summary,
		Details:   value.Details,
	}, nil
}
