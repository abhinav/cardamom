package boardconflict

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestConflictResolveUsesTheReviewedContenderSet(t *testing.T) {
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 2)
	candidates := []Candidate{
		{Origins: []boardhistory.EventID{origins[0]}, Value: &boardhistory.IssueTitleSet{IssueUID: uid, Title: "One"}},
		{Origins: []boardhistory.EventID{origins[1]}, Value: &boardhistory.IssueTitleSet{IssueUID: uid, Title: "Two"}},
	}
	conflict, err := NewConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	alternative := conflict.Alternatives()[0]
	when := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	event, err := conflict.Resolve(ResolveRequest{
		ConflictID: conflict.ID(), Choice: TakeAlternative{ID: alternative.ID},
	}, valueForAlternative(t, candidates, alternative), when)
	require.NoError(t, err)
	assert.Equal(t, valueForAlternative(t, candidates, alternative), event.Payload)
	assert.Equal(t, when, event.OccurredAt)
	_, err = conflict.Resolve(ResolveRequest{
		ConflictID: "conflict_stale", Choice: TakeAlternative{ID: alternative.ID},
	}, valueForAlternative(t, candidates, alternative), when)
	assert.ErrorContains(t, err, "changed")
}

func TestConflictResolveComposesOnlyTextFields(t *testing.T) {
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 2)
	candidates := []Candidate{
		{Origins: []boardhistory.EventID{origins[0]}, Value: &boardhistory.IssueSummarySet{IssueUID: uid, Summary: new("One")}},
		{Origins: []boardhistory.EventID{origins[1]}, Value: &boardhistory.IssueSummarySet{IssueUID: uid, Summary: new("Two")}},
	}
	conflict, err := NewConflict("board-test", candidateSequence(candidates))
	require.NoError(t, err)
	event, err := conflict.Resolve(ResolveRequest{
		ConflictID: conflict.ID(), Choice: ReplaceText{Body: "Both ideas."},
	}, nil, time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, &boardhistory.IssueSummarySet{IssueUID: uid, Summary: new("Both ideas.")}, event.Payload)
}
