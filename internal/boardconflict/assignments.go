package boardconflict

import (
	"fmt"
	"iter"

	"go.abhg.dev/cardamom/internal/boardhistory"
)

// Assignments projects the independently assignable values represented by a
// retained event. Origins supply several values under one event identity;
// ordinary assignments supply one. Logs, attachments, and identity markers
// have separate merge rules and supply none.
// The projected payloads share immutable content with the input.
func Assignments(value boardhistory.Payload) iter.Seq[boardhistory.Payload] {
	return func(yield func(boardhistory.Payload) bool) {
		switch value := value.(type) {
		case *boardhistory.IssueOrigin:
			for _, assignment := range []boardhistory.Payload{
				&boardhistory.IssueTitleSet{IssueUID: value.UID, Title: value.Title},
				&boardhistory.IssueKindSet{IssueUID: value.UID, Kind: value.Kind},
				&boardhistory.IssueLifecycleSet{IssueUID: value.UID, Lifecycle: value.Lifecycle, ClosedAt: value.ClosedAt},
				&boardhistory.IssuePrioritySet{IssueUID: value.UID, Priority: value.Priority},
				&boardhistory.IssueWaitingSet{IssueUID: value.UID, Waiting: value.Waiting},
				&boardhistory.IssueSummarySet{IssueUID: value.UID, Summary: value.Summary},
				&boardhistory.IssueDetailsSet{IssueUID: value.UID, Details: value.Details},
			} {
				if !yield(assignment) {
					return
				}
			}
		case *boardhistory.BoardOrigin:
			for _, assignment := range []boardhistory.Payload{
				&boardhistory.BoardNameSet{Name: value.Name},
				&boardhistory.BoardDescriptionSet{Description: value.Description},
				&boardhistory.BoardArchiveSet{Archive: value.Archive},
				&boardhistory.BoardIssueIDPrefixSet{Prefix: value.IssueIDPrefix},
				&boardhistory.BoardIssueIDStrategySet{Strategy: value.IssueIDStrategy},
				&boardhistory.BoardSummaryLimitSet{Limit: value.SummaryMaxBytes},
				&boardhistory.BoardAttachmentLimitSet{Limit: value.AttachmentMaxBytes},
				&boardhistory.BoardPinLimitSet{Limit: value.PinsMaxCount},
			} {
				if !yield(assignment) {
					return
				}
			}
		default:
			if _, err := UnitOf(value); err == nil {
				yield(value)
			}
		}
	}
}

// ProjectAssignment selects a semantic unit from a retained event. An origin
// can represent several units, so the event identity alone is insufficient to
// identify the value selected by a conflict alternative.
func ProjectAssignment(value boardhistory.Payload, unit Unit) (boardhistory.Payload, error) {
	for assignment := range Assignments(value) {
		candidate, err := UnitOf(assignment)
		if err != nil {
			return nil, err
		}
		if candidate == unit {
			return assignment, nil
		}
	}
	return nil, fmt.Errorf("retained event does not assign %s", unit.Kind)
}
