package conflictconnect

import (
	"encoding/hex"
	"fmt"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	v1 "go.abhg.dev/cardamom/internal/gen/cardamom/private/v1"
	"go.abhg.dev/cardamom/internal/issue"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *Service) summary(id board.ID, value boardconflict.Summary) (*v1.ConflictSummary, error) {
	refs, err := s.Views.References(id, value.Issues)
	if err != nil {
		return nil, err
	}
	out := &v1.ConflictSummary{Id: value.ID, Unit: encodeUnit(value.Unit), Issues: refs, Alternatives: uint32(value.Alternatives)}
	if value.ConstraintKind != 0 {
		out.Constraint = value.ConstraintKind.String()
	}
	return out, nil
}

func (s *Service) inspection(id board.ID, value boardconflict.Inspection) (*v1.ConflictInspection, error) {
	out := &v1.ConflictInspection{Summary: &v1.ConflictSummary{Id: value.Conflict.ID()}}
	var err error
	out.Issues, err = s.references(id, value.Issues)
	if err != nil {
		return nil, err
	}
	for _, ref := range out.Issues {
		out.Summary.Issues = append(out.Summary.Issues, ref.Issue)
	}
	switch conflict := value.Conflict.(type) {
	case *boardconflict.Conflict:
		out.Summary.Unit = encodeUnit(conflict.Unit())
		for _, alternative := range conflict.Alternatives() {
			out.Alternatives = append(out.Alternatives, encodeAlternative(alternative.ID, conflict.Unit(), alternative.Origins))
		}
	case *boardconflict.ConstraintConflict:
		out.Summary.Constraint = conflict.Kind().String()
		for _, assignment := range conflict.Assignments() {
			alternative := encodeAlternative(string(assignment.AlternativeID), assignment.Unit, assignment.Origins)
			alternative.WaitingPresent = assignment.WaitingPresent
			out.Alternatives = append(out.Alternatives, alternative)
		}
	}
	out.Summary.Alternatives = uint32(len(out.Alternatives))
	return out, nil
}

func (s *Service) references(id board.ID, values []boardconflict.IssueReference) ([]*v1.ConflictIssue, error) {
	out := make([]*v1.ConflictIssue, 0, len(values))
	for _, value := range values {
		refs, err := s.Views.References(id, []issue.Reference{value.Reference})
		if err != nil {
			return nil, err
		}
		out = append(out, &v1.ConflictIssue{Uid: hex.EncodeToString(value.UID.Bytes()), Issue: refs[0]})
	}
	return out, nil
}

func encodeAlternative(id string, unit boardconflict.Unit, origins []boardhistory.EventID) *v1.ConflictAlternative {
	out := &v1.ConflictAlternative{Id: id, Unit: encodeUnit(unit)}
	for _, origin := range origins {
		out.Origins = append(out.Origins, &v1.ConflictOrigin{Writer: hex.EncodeToString(origin.Writer.Bytes()), Sequence: origin.Sequence, Ordinal: origin.Ordinal})
	}
	return out
}

func encodeUnit(value boardconflict.Unit) *v1.ConflictUnit {
	out := &v1.ConflictUnit{Kind: string(value.Kind), Key: value.Key}
	if value.IssueUID != (issue.UID{}) {
		out.IssueUid = hex.EncodeToString(value.IssueUID.Bytes())
	}
	if value.RelatedUID != (issue.UID{}) {
		out.RelatedUid = hex.EncodeToString(value.RelatedUID.Bytes())
	}
	return out
}

// encodeValue translates maintained semantic assignments without serializing a
// journal envelope. Optional values retain absence, including inherited settings.
func encodeValue(value boardhistory.Payload) (*v1.ConflictValue, error) {
	unit, err := boardconflict.UnitOf(value)
	if err != nil {
		return nil, err
	}
	out := &v1.ConflictValue{Unit: encodeUnit(unit)}
	switch value := value.(type) {
	case *boardhistory.IssueTitleSet:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: &value.Title}}
	case *boardhistory.BoardNameSet:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: &value.Name}}
	case *boardhistory.BoardDescriptionSet:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: value.Description}}
	case *boardhistory.IssueSummarySet:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: value.Summary}}
	case *boardhistory.IssueDetailsSet:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: value.Details}}
	case *boardhistory.IssueResult:
		out.Value = &v1.ConflictValue_Text{Text: &v1.ConflictText{Text: value.Body}}
	case *boardhistory.IssueKindSet:
		out.Value = &v1.ConflictValue_Kind{Kind: value.Kind}
	case *boardhistory.IssuePrioritySet:
		out.Value = &v1.ConflictValue_Priority{Priority: int32(value.Priority)}
	case *boardhistory.IssueLifecycleSet:
		out.Value = &v1.ConflictValue_Lifecycle{Lifecycle: &v1.ConflictLifecycle{Lifecycle: value.Lifecycle, ClosedAt: optionalTime(value.ClosedAt)}}
	case *boardhistory.IssueWaitingSet:
		waiting := &v1.ConflictWaiting{}
		if value.Waiting != nil {
			waiting.Reason = &value.Waiting.Reason
			waiting.Since = timestamppb.New(value.Waiting.Since)
		}
		out.Value = &v1.ConflictValue_Waiting{Waiting: waiting}
	case *boardhistory.IssueState:
		state := &v1.ConflictState{}
		if value.State != nil {
			state.Body = &value.State.Body
			state.Author = value.State.Author
			state.UpdatedAt = optionalTime(value.State.UpdatedAt)
			state.NextAction = value.State.NextAction
			state.SnapshotLogEntryId = value.State.SnapshotLogEntryID
		}
		out.Value = &v1.ConflictValue_State{State: state}
	case *boardhistory.IssueCheckpoint:
		out.Value = &v1.ConflictValue_Checkpoint{Checkpoint: &v1.ConflictCheckpoint{Outcome: value.Outcome, DecidedAt: timestamppb.New(value.DecidedAt), Reason: value.Reason}}
	case *boardhistory.IssueLabel:
		out.Value = &v1.ConflictValue_Membership{Membership: &v1.ConflictMembership{Present: value.Present, IssueUid: hex.EncodeToString(value.IssueUID.Bytes())}}
	case *boardhistory.IssueDependency:
		out.Value = &v1.ConflictValue_Membership{Membership: &v1.ConflictMembership{Present: value.Present, IssueUid: hex.EncodeToString(value.IssueUID.Bytes()), RelatedUid: hex.EncodeToString(value.PrerequisiteUID.Bytes())}}
	case *boardhistory.IssueContainment:
		out.Value = &v1.ConflictValue_Membership{Membership: &v1.ConflictMembership{Present: value.Present, IssueUid: hex.EncodeToString(value.ChildUID.Bytes()), RelatedUid: hex.EncodeToString(value.ParentUID.Bytes())}}
	case *boardhistory.ExternalKey:
		out.Value = &v1.ConflictValue_Membership{Membership: &v1.ConflictMembership{Present: value.Present, IssueUid: hex.EncodeToString(value.IssueUID.Bytes())}}
	case *boardhistory.BoardPin:
		out.Value = &v1.ConflictValue_Pin{Pin: &v1.ConflictPin{Present: value.Present, Position: value.Position}}
	case *boardhistory.BoardArchiveSet:
		archive := &v1.ConflictArchive{}
		if value.Archive != nil {
			archive.Actor = &value.Archive.Actor
			archive.At = timestamppb.New(value.Archive.At)
		}
		out.Value = &v1.ConflictValue_Archive{Archive: archive}
	case *boardhistory.BoardIssueIDPrefixSet:
		out.Value = &v1.ConflictValue_OverrideText{OverrideText: &v1.ConflictText{Text: value.Prefix}}
	case *boardhistory.BoardIssueIDStrategySet:
		out.Value = &v1.ConflictValue_OverrideText{OverrideText: &v1.ConflictText{Text: value.Strategy}}
	case *boardhistory.BoardSummaryLimitSet:
		out.Value = &v1.ConflictValue_OverrideLimit{OverrideLimit: &v1.ConflictLimit{Value: value.Limit}}
	case *boardhistory.BoardAttachmentLimitSet:
		out.Value = &v1.ConflictValue_OverrideLimit{OverrideLimit: &v1.ConflictLimit{Value: value.Limit}}
	case *boardhistory.BoardPinLimitSet:
		out.Value = &v1.ConflictValue_OverrideLimit{OverrideLimit: &v1.ConflictLimit{Value: value.Limit}}
	default:
		return nil, fmt.Errorf("unsupported conflict assignment %T", value)
	}
	return out, nil
}

func optionalTime(value *time.Time) *timestamppb.Timestamp {
	if value == nil {
		return nil
	}
	return timestamppb.New(*value)
}
