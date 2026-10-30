package cli

import (
	"encoding/hex"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

// Conflict output uses public references for navigation and hexadecimal private
// identities for provenance. Domain UID internals are deliberately not JSON.
type conflictUnitOutput struct {
	Kind       string `json:"kind"`
	IssueUID   string `json:"issue_uid,omitempty"`
	RelatedUID string `json:"related_uid,omitempty"`
	Key        string `json:"key,omitempty"`
}

type conflictSummaryOutput struct {
	ID           string             `json:"id"`
	Unit         conflictUnitOutput `json:"unit"`
	Constraint   string             `json:"constraint,omitempty"`
	Issues       []issue.Reference  `json:"issues"`
	Alternatives int                `json:"alternatives"`
}

type conflictPageOutput struct {
	Conflicts []conflictSummaryOutput `json:"conflicts"`
	Next      string                  `json:"next"`
}

func conflictPageJSON(page boardconflict.Page) conflictPageOutput {
	out := conflictPageOutput{Conflicts: make([]conflictSummaryOutput, 0, len(page.Conflicts)), Next: page.Next}
	for _, value := range page.Conflicts {
		out.Conflicts = append(out.Conflicts, conflictSummaryOutput{ID: value.ID, Unit: conflictUnitJSON(value.Unit), Constraint: value.ConstraintKind.String(), Issues: value.Issues, Alternatives: value.Alternatives})
	}
	return out
}

func conflictUnitJSON(value boardconflict.Unit) conflictUnitOutput {
	out := conflictUnitOutput{Kind: string(value.Kind), Key: value.Key}
	if value.IssueUID != (issue.UID{}) {
		out.IssueUID = hex.EncodeToString(value.IssueUID.Bytes())
	}
	if value.RelatedUID != (issue.UID{}) {
		out.RelatedUID = hex.EncodeToString(value.RelatedUID.Bytes())
	}
	return out
}

type conflictOriginOutput struct {
	Writer   string `json:"writer"`
	Sequence uint64 `json:"sequence"`
	Ordinal  uint64 `json:"ordinal"`
}

type conflictAlternativeOutput struct {
	ID             string                 `json:"id"`
	Unit           conflictUnitOutput     `json:"unit"`
	Origins        []conflictOriginOutput `json:"origins"`
	WaitingPresent bool                   `json:"waiting_present,omitempty"`
}

type conflictIssueOutput struct {
	UID   string          `json:"uid"`
	Issue issue.Reference `json:"issue"`
}

type conflictInspectionOutput struct {
	ID           string                      `json:"id"`
	Unit         conflictUnitOutput          `json:"unit"`
	Constraint   string                      `json:"constraint,omitempty"`
	Alternatives []conflictAlternativeOutput `json:"alternatives"`
	Issues       []conflictIssueOutput       `json:"issues"`
}

func conflictInspectionJSON(value boardconflict.Inspection) conflictInspectionOutput {
	out := conflictInspectionOutput{ID: value.Conflict.ID(), Issues: conflictIssuesJSON(value.Issues)}
	switch conflict := value.Conflict.(type) {
	case *boardconflict.Conflict:
		out.Unit = conflictUnitJSON(conflict.Unit())
		for _, alternative := range conflict.Alternatives() {
			out.Alternatives = append(out.Alternatives, conflictAlternativeJSON(alternative.ID, conflict.Unit(), alternative.Origins))
		}
	case *boardconflict.ConstraintConflict:
		out.Constraint = conflict.Kind().String()
		for _, assignment := range conflict.Assignments() {
			alternative := conflictAlternativeJSON(string(assignment.AlternativeID), assignment.Unit, assignment.Origins)
			alternative.WaitingPresent = assignment.WaitingPresent
			out.Alternatives = append(out.Alternatives, alternative)
		}
	}
	return out
}

func conflictAlternativeJSON(id string, unit boardconflict.Unit, origins []boardhistory.EventID) conflictAlternativeOutput {
	out := conflictAlternativeOutput{ID: id, Unit: conflictUnitJSON(unit)}
	for _, origin := range origins {
		out.Origins = append(out.Origins, conflictOriginOutput{Writer: hex.EncodeToString(origin.Writer.Bytes()), Sequence: origin.Sequence, Ordinal: origin.Ordinal})
	}
	return out
}

func conflictIssuesJSON(values []boardconflict.IssueReference) []conflictIssueOutput {
	out := make([]conflictIssueOutput, 0, len(values))
	for _, value := range values {
		out = append(out, conflictIssueOutput{UID: hex.EncodeToString(value.UID.Bytes()), Issue: value.Reference})
	}
	return out
}

type conflictValueOutput struct {
	Unit   conflictUnitOutput    `json:"unit"`
	Value  any                   `json:"value"`
	Issues []conflictIssueOutput `json:"issues"`
}

// conflictValueJSON preserves field types and absence without exposing journal
// envelopes or requiring callers to parse the human display.
func conflictValueJSON(value boardconflict.Value) (conflictValueOutput, error) {
	unit, err := boardconflict.UnitOf(value.Payload)
	if err != nil {
		return conflictValueOutput{}, err
	}
	out := conflictValueOutput{Unit: conflictUnitJSON(unit), Issues: conflictIssuesJSON(value.Issues)}
	switch body := value.Payload.(type) {
	case *boardhistory.IssueTitleSet:
		out.Value = body.Title
	case *boardhistory.BoardNameSet:
		out.Value = body.Name
	case *boardhistory.IssueSummarySet:
		out.Value = body.Summary
	case *boardhistory.IssueDetailsSet:
		out.Value = body.Details
	case *boardhistory.BoardDescriptionSet:
		out.Value = body.Description
	case *boardhistory.IssueResult:
		out.Value = body.Body
	case *boardhistory.IssueKindSet:
		out.Value = body.Kind
	case *boardhistory.IssuePrioritySet:
		out.Value = body.Priority
	case *boardhistory.BoardIssueIDPrefixSet:
		out.Value = body.Prefix
	case *boardhistory.BoardIssueIDStrategySet:
		out.Value = body.Strategy
	case *boardhistory.BoardSummaryLimitSet:
		out.Value = body.Limit
	case *boardhistory.BoardAttachmentLimitSet:
		out.Value = body.Limit
	case *boardhistory.BoardPinLimitSet:
		out.Value = body.Limit
	case *boardhistory.IssueLifecycleSet:
		out.Value = conflictLifecycleOutput{Lifecycle: body.Lifecycle, ClosedAt: body.ClosedAt}
	case *boardhistory.IssueWaitingSet:
		if body.Waiting != nil {
			out.Value = conflictWaitingOutput{Reason: body.Waiting.Reason, Since: body.Waiting.Since}
		}
	case *boardhistory.IssueState:
		if body.State != nil {
			out.Value = conflictStateOutput{Body: body.State.Body, Author: body.State.Author, UpdatedAt: body.State.UpdatedAt, SnapshotLogEntryID: body.State.SnapshotLogEntryID, NextAction: body.State.NextAction}
		}
	case *boardhistory.IssueCheckpoint:
		out.Value = conflictCheckpointOutput{Outcome: body.Outcome, Reason: body.Reason, DecidedAt: body.DecidedAt}
	case *boardhistory.IssueLabel:
		out.Value = conflictMembershipOutput{Present: body.Present, IssueUID: hex.EncodeToString(body.IssueUID.Bytes())}
	case *boardhistory.IssueDependency:
		out.Value = conflictMembershipOutput{Present: body.Present, IssueUID: hex.EncodeToString(body.IssueUID.Bytes()), RelatedUID: hex.EncodeToString(body.PrerequisiteUID.Bytes())}
	case *boardhistory.IssueContainment:
		out.Value = conflictMembershipOutput{Present: body.Present, IssueUID: hex.EncodeToString(body.ChildUID.Bytes()), RelatedUID: hex.EncodeToString(body.ParentUID.Bytes())}
	case *boardhistory.ExternalKey:
		out.Value = conflictMembershipOutput{Present: body.Present, IssueUID: hex.EncodeToString(body.IssueUID.Bytes())}
	case *boardhistory.BoardPin:
		out.Value = conflictPinOutput{Present: body.Present, Position: body.Position}
	case *boardhistory.BoardArchiveSet:
		if body.Archive != nil {
			out.Value = conflictArchiveOutput{Actor: body.Archive.Actor, At: body.Archive.At}
		}
	default:
		return out, fmt.Errorf("unsupported conflict assignment %T", value.Payload)
	}
	return out, nil
}

type conflictLifecycleOutput struct {
	Lifecycle string     `json:"lifecycle"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
}
type conflictWaitingOutput struct {
	Reason string    `json:"reason"`
	Since  time.Time `json:"since"`
}
type conflictStateOutput struct {
	Body               string     `json:"body"`
	Author             *string    `json:"author,omitempty"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
	SnapshotLogEntryID *string    `json:"snapshot_log_entry_id,omitempty"`
	NextAction         *string    `json:"next_action,omitempty"`
}
type conflictCheckpointOutput struct {
	Outcome   string    `json:"outcome"`
	Reason    string    `json:"reason"`
	DecidedAt time.Time `json:"decided_at"`
}
type conflictMembershipOutput struct {
	Present    bool   `json:"present"`
	IssueUID   string `json:"issue_uid"`
	RelatedUID string `json:"related_uid,omitempty"`
}
type conflictPinOutput struct {
	Present  bool  `json:"present"`
	Position int64 `json:"position"`
}
type conflictArchiveOutput struct {
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}
type conflictResolutionOutput struct {
	ConflictID string `json:"conflict_id"`
	Revision   int64  `json:"revision"`
}
