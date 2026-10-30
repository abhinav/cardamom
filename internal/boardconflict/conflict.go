// Package boardconflict owns unresolved board edits and their explicit
// resolution. Alternatives retain ordinary semantic history values and event
// identities. The board repository owns their transactional publication;
// callers inspect or resolve finite conflicts without handling storage scopes.
package boardconflict

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

// Conflict retains mutually incompatible current values of one semantic unit.
// Its identity binds the complete displayed contender set, so a resolution
// cannot silently supersede a value that arrived after the caller inspected it.
type Conflict struct {
	id           string
	boardID      board.ID
	unit         Unit
	alternatives []Alternative
	affected     []AffectedIssue
}

// Candidate is one current value and every causally maximal assignment that
// supports it. The caller establishes maximality from retained frontiers.
type Candidate struct {
	// Origins identifies the assignments that have not been superseded.
	Origins []boardhistory.EventID // required

	// Value is one typed assignment from the retained history vocabulary.
	Value boardhistory.Payload // required
}

// NewConflict retains candidates for one unit and coalesces equal values.
// It rejects mixed units, duplicate event identities with different values,
// and fewer than two distinct values. Only compact metadata is retained;
// each input body can be released before reading the next candidate.
func NewConflict(boardID board.ID, candidates iter.Seq2[Candidate, error]) (*Conflict, error) {
	conflict := &Conflict{boardID: boardID}
	groups := make(map[[32]byte]int)
	var groupOrigins [][]boardhistory.EventID
	origins := make(map[boardhistory.EventID][32]byte)
	affected := make(map[issue.UID]struct{})
	for candidate, err := range candidates {
		if err != nil {
			return nil, err
		}
		encoded, err := encodeValue(candidate.Value)
		if err != nil {
			return nil, fmt.Errorf("candidate value: %w", err)
		}
		unit, err := UnitOf(candidate.Value)
		if err != nil {
			return nil, err
		}
		if conflict.unit.Kind == "" {
			conflict.unit = unit
		} else if unit != conflict.unit {
			return nil, errors.New("conflict candidates must address the same unit")
		}
		if len(candidate.Origins) == 0 {
			return nil, errors.New("candidate requires originating events")
		}
		key := semanticKey(candidate.Value, encoded)
		group, found := groups[key]
		if !found {
			group = len(groupOrigins)
			groups[key] = group
			groupOrigins = append(groupOrigins, nil)
		}
		if owner, ok := candidate.Value.(*boardhistory.ExternalKey); ok && owner.Present {
			affected[owner.IssueUID] = struct{}{}
		}
		for _, origin := range candidate.Origins {
			if origin.Writer == (board.WriterUID{}) || origin.Sequence == 0 {
				return nil, errors.New("candidate event requires writer and sequence")
			}
			if prior, found := origins[origin]; found {
				if prior != key {
					return nil, errors.New("one event identifies incompatible candidate values")
				}
				continue
			}
			origins[origin] = key
			groupOrigins[group] = append(groupOrigins[group], origin)
		}
	}
	if len(groupOrigins) < 2 {
		return nil, errors.New("conflict requires at least two distinct values")
	}
	for _, origins := range groupOrigins {
		slices.SortFunc(origins, compareEventID)
		conflict.alternatives = append(conflict.alternatives, Alternative{
			ID: digestID("alt_", eventIdentityBytes(origins)), Origins: origins,
		})
	}
	slices.SortFunc(conflict.alternatives, func(left, right Alternative) int {
		return bytes.Compare([]byte(left.ID), []byte(right.ID))
	})
	identity := append([]byte(boardID.String()+"\x00"), conflict.unit.identityBytes()...)
	for _, alternative := range conflict.alternatives {
		identity = append(identity, []byte(alternative.ID)...)
	}
	conflict.id = digestID("conflict_", identity)
	for uid := range affected {
		conflict.affected = append(conflict.affected, AffectedIssue{UID: uid, Field: issue.FieldExternalKeys})
	}
	slices.SortFunc(conflict.affected, func(left, right AffectedIssue) int { return bytes.Compare(left.UID.Bytes(), right.UID.Bytes()) })
	return conflict, nil
}

// Snapshot is persisted, already validated conflict metadata. Repository
// readers load it separately from retained candidate bodies.
type Snapshot struct {
	// ID binds the current contender set.
	ID string // required
	// BoardID identifies the logical board.
	BoardID board.ID // required
	// Unit identifies the unavailable field.
	Unit Unit // required
	// Alternatives contains identities and origins, never candidate bodies.
	Alternatives []Alternative // required
	// Affected contains competing producer-key owners when Unit has no issue.
	Affected []AffectedIssue
}

// Load restores metadata established by a supported writer. It owns copies of
// its slices; it does not reread or rehash immutable bodies to reprove grouping.
func Load(snapshot Snapshot) *Conflict {
	conflict := &Conflict{id: snapshot.ID, boardID: snapshot.BoardID, unit: snapshot.Unit, affected: slices.Clone(snapshot.Affected)}
	conflict.alternatives = slices.Clone(snapshot.Alternatives)
	for i := range conflict.alternatives {
		conflict.alternatives[i].Origins = slices.Clone(snapshot.Alternatives[i].Origins)
	}
	return conflict
}

// ID is the review token for this subject and contender set.
func (c *Conflict) ID() string { return c.id }

// BoardID identifies the logical board whose state is disputed.
func (c *Conflict) BoardID() board.ID { return c.boardID }

// Unit identifies the field or collection member requiring resolution.
func (c *Conflict) Unit() Unit { return c.unit }

// Alternatives returns compact value identities in stable display order.
// Origins are copied; callers retrieve each body separately.
func (c *Conflict) Alternatives() []Alternative {
	out := slices.Clone(c.alternatives)
	for index := range out {
		out[index].Origins = slices.Clone(out[index].Origins)
	}
	return out
}

// AffectedIssues reports compact issue absence metadata for this conflict.
// Board settings have no issue subjects; producer-key ownership can affect
// several competing owners. Candidate bodies remain in the detail record.
func (c *Conflict) AffectedIssues() []AffectedIssue {
	var field issue.Field
	switch c.unit.Kind {
	case boardhistory.EventIssueTitleSet:
		field = issue.FieldTitle
	case boardhistory.EventIssueKindSet:
		field = issue.FieldKind
	case boardhistory.EventIssueLifecycleSet:
		field = issue.FieldLifecycle
	case boardhistory.EventIssuePrioritySet:
		field = issue.FieldPriority
	case boardhistory.EventIssueWaitingSet:
		field = issue.FieldWaiting
	case boardhistory.EventIssueSummarySet:
		field = issue.FieldSummary
	case boardhistory.EventIssueDetailsSet:
		field = issue.FieldDetails
	case boardhistory.EventIssueState:
		field = issue.FieldState
	case boardhistory.EventIssueResult:
		field = issue.FieldResult
	case boardhistory.EventIssueCheckpoint:
		field = issue.FieldCheckpoint
	case boardhistory.EventIssueLabel:
		field = issue.FieldLabels
	case boardhistory.EventIssueDependency:
		field = issue.FieldDependencies
	case boardhistory.EventIssueContainment:
		field = issue.FieldParent
	case boardhistory.EventBoardPin:
		field = issue.FieldPins
	case boardhistory.EventExternalKey:
		return slices.Clone(c.affected)
	default:
		return nil
	}
	return []AffectedIssue{{UID: c.unit.IssueUID, Field: field}}
}

// Alternative identifies one distinct value and the assignments supporting it.
// Origins are sorted by event identity. Their first member identifies a stable
// representative retained body, including semantically equal removals.
type Alternative struct {
	// ID selects this value within the displayed conflict.
	ID string // required

	// Origins preserves every equal concurrent assignment.
	Origins []boardhistory.EventID // required
}

// Unit identifies one independently assignable value within a board.
// Kind selects the retained assignment vocabulary. IssueUID identifies issue
// fields, RelatedUID identifies a prerequisite, and Key identifies a label or
// external key. Parent assignments are keyed only by the child issue.
type Unit struct {
	// Kind identifies the semantic assignment family.
	Kind boardhistory.EventKind // required

	// IssueUID identifies the issue, or is zero for board fields and external keys.
	IssueUID issue.UID

	// RelatedUID identifies a dependency's prerequisite.
	RelatedUID issue.UID

	// Key identifies the collection member for labels or producer keys.
	Key string
}

// UnitOf identifies the value assigned by one supported mutable payload.
// Origin, attachment, Log, and reference-marker events have separate merge
// rules and are not scalar conflict alternatives.
func UnitOf(value boardhistory.Payload) (Unit, error) {
	unit := Unit{Kind: boardhistory.PayloadKind(value)}
	switch value := value.(type) {
	case *boardhistory.BoardNameSet, *boardhistory.BoardDescriptionSet,
		*boardhistory.BoardArchiveSet, *boardhistory.BoardIssueIDPrefixSet,
		*boardhistory.BoardIssueIDStrategySet, *boardhistory.BoardSummaryLimitSet,
		*boardhistory.BoardAttachmentLimitSet, *boardhistory.BoardPinLimitSet:
	case *boardhistory.IssueTitleSet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueKindSet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueLifecycleSet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssuePrioritySet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueWaitingSet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueSummarySet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueDetailsSet:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueState:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueResult:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueCheckpoint:
		unit.IssueUID = value.IssueUID
	case *boardhistory.IssueLabel:
		unit.IssueUID, unit.Key = value.IssueUID, value.Label
	case *boardhistory.IssueDependency:
		unit.IssueUID, unit.RelatedUID = value.IssueUID, value.PrerequisiteUID
	case *boardhistory.IssueContainment:
		unit.IssueUID = value.ChildUID
	case *boardhistory.ExternalKey:
		unit.Key = value.Key
	case *boardhistory.BoardPin:
		unit.IssueUID = value.IssueUID
	default:
		return Unit{}, fmt.Errorf("event kind %q cannot be a field conflict", unit.Kind)
	}
	return unit, nil
}

func (u Unit) identityBytes() []byte {
	value := append([]byte(u.Kind+"\x00"), u.IssueUID.Bytes()...)
	value = append(value, u.RelatedUID.Bytes()...)
	return append(value, []byte(u.Key)...)
}

func encodeValue(value boardhistory.Payload) ([]byte, error) {
	// Assignment time is provenance, not field content. A fixed envelope time
	// lets the journal codec compare and retain typed values without adding
	// another persistent value vocabulary.
	return boardhistory.MarshalEvent(boardhistory.Event{
		OccurredAt: time.Unix(1, 0).UTC(), Payload: value,
	})
}

func semanticKey(value boardhistory.Payload, encoded []byte) [32]byte {
	// Removing containment or a producer key assigns absence. The old target
	// identifies the removed edge, not a different resulting absent value.
	switch value := value.(type) {
	case *boardhistory.IssueContainment:
		if !value.Present {
			return sha256.Sum256([]byte("parent absent"))
		}
	case *boardhistory.ExternalKey:
		if !value.Present {
			return sha256.Sum256([]byte("external key absent"))
		}
	}
	return sha256.Sum256(encoded)
}

func eventIdentityBytes(origins []boardhistory.EventID) []byte {
	encoded := make([]byte, 0, len(origins)*32)
	for _, origin := range origins {
		encoded = append(encoded, origin.Writer.Bytes()...)
		encoded = binary.BigEndian.AppendUint64(encoded, origin.Sequence)
		encoded = binary.BigEndian.AppendUint64(encoded, origin.Ordinal)
	}
	return encoded
}

func compareEventID(left, right boardhistory.EventID) int {
	return bytes.Compare(eventIdentityBytes([]boardhistory.EventID{left}), eventIdentityBytes([]boardhistory.EventID{right}))
}

func digestID(prefix string, value []byte) string {
	digest := sha256.Sum256(value)
	return prefix + hex.EncodeToString(digest[:])
}

// AffectedIssue identifies an issue field whose current value is disputed.
// It is immutable metadata and can be copied independently.
type AffectedIssue struct {
	// UID is the private identity used by projections and retained history.
	UID issue.UID // required
	// Field identifies the unavailable current issue fact.
	Field issue.Field // required
}
