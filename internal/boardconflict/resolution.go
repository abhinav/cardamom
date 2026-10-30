package boardconflict

import (
	"strings"
	"time"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/errkind"
)

// ResolveRequest supplies the conflict the caller reviewed and one explicit
// resolution. The conflict identity is the review token, not merely a field ID.
type ResolveRequest struct {
	// ConflictID must still identify the current contender set.
	ConflictID string // required

	// Choice selects one alternative or composes supported text.
	Choice Choice // required
}

// Choice is the closed set of field-resolution requests.
// Relationship repairs use the separate structural resolution operation.
type Choice interface{ conflictChoice() }

// TakeAlternative selects one value from the reviewed conflict.
// It is an immutable request value and may be copied independently.
type TakeAlternative struct {
	// ID identifies the displayed alternative.
	ID string // required
}

func (TakeAlternative) conflictChoice() {}

// ReplaceText composes a new title, name, Summary, Details, or Result.
// It is an immutable request value. Empty text clears optional text fields.
type ReplaceText struct {
	// Body contains the complete replacement text.
	Body string
}

func (ReplaceText) conflictChoice() {}

// Resolve produces the ordinary semantic assignment selected by request.
// The repository appends it with the current observed frontier and updates
// projections in one transaction. A changed review token or unavailable
// alternative returns Conflict without a replacement assignment. selectedValue
// is the repository-owned retained body for a selected alternative; text
// composition passes nil and does not fetch an old body.
func (c *Conflict) Resolve(request ResolveRequest, selectedValue boardhistory.Payload, occurredAt time.Time) (boardhistory.Event, error) {
	if request.ConflictID != c.id {
		return boardhistory.Event{}, errkind.Errorf(errkind.Conflict,
			"conflict changed; inspect the current alternatives")
	}
	var value boardhistory.Payload
	switch choice := request.Choice.(type) {
	case TakeAlternative:
		for _, alternative := range c.alternatives {
			if alternative.ID == choice.ID {
				value = selectedValue
				break
			}
		}
		if value == nil {
			return boardhistory.Event{}, errkind.Errorf(errkind.Conflict,
				"alternative %q is not in conflict %q; inspect the conflict again", choice.ID, c.id)
		}
	case ReplaceText:
		body := strings.TrimSpace(choice.Body)
		var optional *string
		if body != "" {
			optional = &body
		}
		switch c.unit.Kind {
		case boardhistory.EventIssueTitleSet:
			value = &boardhistory.IssueTitleSet{IssueUID: c.unit.IssueUID, Title: body}
		case boardhistory.EventIssueSummarySet:
			value = &boardhistory.IssueSummarySet{IssueUID: c.unit.IssueUID, Summary: optional}
		case boardhistory.EventIssueDetailsSet:
			value = &boardhistory.IssueDetailsSet{IssueUID: c.unit.IssueUID, Details: optional}
		case boardhistory.EventIssueResult:
			value = &boardhistory.IssueResult{IssueUID: c.unit.IssueUID, Body: optional}
		case boardhistory.EventBoardNameSet:
			value = &boardhistory.BoardNameSet{Name: body}
		case boardhistory.EventBoardDescriptionSet:
			value = &boardhistory.BoardDescriptionSet{Description: optional}
		default:
			return boardhistory.Event{}, errkind.Errorf(errkind.InvalidInput,
				"%s does not accept a text replacement; select an alternative", c.unit.Kind)
		}
	default:
		return boardhistory.Event{}, errkind.Errorf(errkind.InvalidInput, "conflict resolution choice required")
	}
	event := boardhistory.Event{OccurredAt: occurredAt, Payload: value}
	if err := boardhistory.ValidateEvent(event); err != nil {
		return boardhistory.Event{}, errkind.Wrap(errkind.InvalidInput, err)
	}
	return event, nil
}
