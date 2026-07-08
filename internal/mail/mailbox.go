package mail

import (
	"errors"
	"time"
)

// Selection is a validated mailbox query.
type Selection struct {
	recipient     string
	allRecipients bool
	includeRead   bool
	since         time.Time
	limit         int
}

// SelectionRequest describes one mailbox query using a relative age window.
type SelectionRequest struct {
	// Recipient identifies one mailbox. It must be empty when AllRecipients is
	// true and non-empty otherwise.
	Recipient string

	// AllRecipients selects every mailbox without changing read state.
	AllRecipients bool

	// IncludeRead includes messages already consumed by their recipient.
	IncludeRead bool

	// MaxAge limits messages to those created within this duration before the
	// supplied current time. Zero has no age limit.
	MaxAge time.Duration

	// Limit caps the selected message count. Zero has no count limit.
	Limit int
}

// NewSelection parses a relative mailbox selection at one supplied instant.
func NewSelection(request SelectionRequest, now time.Time) (Selection, error) {
	switch {
	case request.AllRecipients && request.Recipient != "":
		return Selection{}, errors.New("recipient must be empty for an all-recipient query")
	case !request.AllRecipients && request.Recipient == "":
		return Selection{}, errors.New("recipient required")
	case request.MaxAge < 0:
		return Selection{}, errors.New("maximum age must be greater than or equal to zero")
	case request.Limit < 0:
		return Selection{}, errors.New("limit must be greater than or equal to zero")
	}

	var since time.Time
	if request.MaxAge > 0 {
		since = now.Add(-request.MaxAge)
	}
	return Selection{
		recipient: request.Recipient, allRecipients: request.AllRecipients,
		includeRead: request.IncludeRead, since: since, limit: request.Limit,
	}, nil
}

// Recipient returns the selected mailbox, or an empty string for all mailboxes.
func (s Selection) Recipient() string { return s.recipient }

// AllRecipients reports whether the selection spans every mailbox.
func (s Selection) AllRecipients() bool { return s.allRecipients }

// IncludeRead reports whether previously consumed messages are selected.
func (s Selection) IncludeRead() bool { return s.includeRead }

// Since returns the inclusive message creation-time floor.
func (s Selection) Since() time.Time { return s.since }

// Limit returns the maximum selected message count. Zero means no cap.
func (s Selection) Limit() int { return s.limit }

// Consumption is a validated read-state transition for one mailbox selection.
type Consumption struct {
	selection Selection
	clear     bool
}

// Read returns a consuming operation that returns the selected messages.
func Read(selection Selection) (Consumption, error) {
	return newConsumption(selection, false)
}

// Clear returns a consuming operation that reports only the unread count.
func Clear(selection Selection) (Consumption, error) {
	return newConsumption(selection, true)
}

func newConsumption(selection Selection, clearOnly bool) (Consumption, error) {
	if selection.AllRecipients() {
		return Consumption{}, errors.New("all-recipient mailbox reads cannot consume messages")
	}
	return Consumption{selection: selection, clear: clearOnly}, nil
}

// Selection returns the messages eligible for this consumption.
func (c Consumption) Selection() Selection { return c.selection }

// ClearOnly reports whether the operation suppresses selected messages and
// returns only the number newly marked as read.
func (c Consumption) ClearOnly() bool { return c.clear }

// Consumed is the committed result of a mailbox consumption.
type Consumed struct {
	// Messages contains selected rows for a read operation and is empty for a
	// clear operation.
	Messages []Message

	// Cleared is the number of unread messages consumed by a clear operation.
	Cleared int
}
