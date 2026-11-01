package boardhistory

import (
	"errors"
	"fmt"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/issue"
)

// ReferenceMarker records collision evidence for a retired public ID.
// Each affected board retains all participants, including other boards, so
// copying either board preserves the ambiguity of existing text references.
// Subsequent events add candidates; they never remove earlier evidence.
type ReferenceMarker struct {
	// PublicID is the permanently retired public handle.
	PublicID string // required

	// Candidates identifies at least two distinct issues that used PublicID.
	Candidates []ReferenceCandidate // required
}

// ReferenceCandidate is one immutable participant in collision evidence.
type ReferenceCandidate struct {
	// IssueUID identifies the participant independently of its public handle.
	IssueUID issue.UID // required

	// BoardID identifies its logical board, which need not be present locally.
	BoardID board.ID // required
}

// EventReferenceMarker records permanent public-ID collision evidence.
const EventReferenceMarker EventKind = "issue.reference_marker"

func (*ReferenceMarker) kind() EventKind { return EventReferenceMarker }

func (m *ReferenceMarker) validateV1() error {
	if err := validateIssueIDV1(m.PublicID); err != nil {
		return err
	}
	if len(m.Candidates) < 2 {
		return errors.New("reference marker requires at least two candidates")
	}
	seen := make(map[issue.UID]struct{}, len(m.Candidates))
	for _, candidate := range m.Candidates {
		if err := requireIssue(candidate.IssueUID); err != nil {
			return err
		}
		if candidate.BoardID == "" {
			return errors.New("reference candidate requires a board identity")
		}
		if _, exists := seen[candidate.IssueUID]; exists {
			return errors.New("reference marker repeats an issue identity")
		}
		seen[candidate.IssueUID] = struct{}{}
	}
	return nil
}

func (m *ReferenceMarker) valueV1() any {
	value := referenceMarkerV1{PublicID: m.PublicID, Candidates: make([]referenceCandidateV1, 0, len(m.Candidates))}
	for _, candidate := range m.Candidates {
		value.Candidates = append(value.Candidates, referenceCandidateV1{
			IssueUID: candidate.IssueUID.Bytes(), BoardID: candidate.BoardID.String(),
		})
	}
	return value
}

type referenceMarkerV1 struct {
	PublicID   string                 `json:"public_id"`
	Candidates []referenceCandidateV1 `json:"candidates"`
}

type referenceCandidateV1 struct {
	IssueUID []byte `json:"issue_uid"`
	BoardID  string `json:"board_id"`
}

func decodeReferenceMarkerV1(encoded []byte) (Payload, error) {
	value, err := decodeValueV1[referenceMarkerV1](encoded)
	if err != nil {
		return nil, err
	}
	marker := &ReferenceMarker{PublicID: value.PublicID, Candidates: make([]ReferenceCandidate, 0, len(value.Candidates))}
	for _, candidate := range value.Candidates {
		uid, err := issue.ParseUID(candidate.IssueUID)
		if err != nil {
			return nil, fmt.Errorf("decode reference candidate: %w", err)
		}
		marker.Candidates = append(marker.Candidates, ReferenceCandidate{
			IssueUID: uid, BoardID: board.ID(candidate.BoardID),
		})
	}
	return marker, nil
}
