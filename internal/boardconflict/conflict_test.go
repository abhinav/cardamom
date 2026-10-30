package boardconflict

import (
	"bytes"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestNewConflictCoalescesEqualValuesWithoutLosingProvenance(t *testing.T) {
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 3)
	first, second, third := origins[0], origins[1], origins[2]
	conflict, err := NewConflict("board-test", candidateSequence([]Candidate{
		{Origins: []boardhistory.EventID{first}, Value: &boardhistory.IssueTitleSet{IssueUID: uid, Title: "One"}},
		{Origins: []boardhistory.EventID{second}, Value: &boardhistory.IssueTitleSet{IssueUID: uid, Title: "One"}},
		{Origins: []boardhistory.EventID{third}, Value: &boardhistory.IssueTitleSet{IssueUID: uid, Title: "Two"}},
	}))
	require.NoError(t, err)
	require.Len(t, conflict.Alternatives(), 2)
	var retained []boardhistory.EventID
	for _, alternative := range conflict.Alternatives() {
		retained = append(retained, alternative.Origins...)
	}
	assert.ElementsMatch(t, []boardhistory.EventID{first, second, third}, retained)
}

func TestConflictReviewTokenIgnoresInputOrderAndChangesWithContenders(t *testing.T) {
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 3)
	one := Candidate{
		Origins: []boardhistory.EventID{origins[0]},
		Value:   &boardhistory.IssueTitleSet{IssueUID: uid, Title: "One"},
	}
	two := Candidate{
		Origins: []boardhistory.EventID{origins[1]},
		Value:   &boardhistory.IssueTitleSet{IssueUID: uid, Title: "Two"},
	}
	first, err := NewConflict("board-test", candidateSequence([]Candidate{one, two}))
	require.NoError(t, err)
	second, err := NewConflict("board-test", candidateSequence([]Candidate{two, one}))
	require.NoError(t, err)
	assert.Equal(t, first.ID(), second.ID())
	third, err := NewConflict("board-test", candidateSequence([]Candidate{one, two, {
		Origins: []boardhistory.EventID{origins[2]},
		Value:   &boardhistory.IssueTitleSet{IssueUID: uid, Title: "Three"},
	}}))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID(), third.ID())
}

func TestNewConflictRejectsDifferentSubjects(t *testing.T) {
	first, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	second, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{2}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 2)
	_, err = NewConflict("board-test", candidateSequence([]Candidate{
		{Origins: []boardhistory.EventID{origins[0]}, Value: &boardhistory.IssueTitleSet{IssueUID: first, Title: "One"}},
		{Origins: []boardhistory.EventID{origins[1]}, Value: &boardhistory.IssueTitleSet{IssueUID: second, Title: "Two"}},
	}))
	assert.ErrorContains(t, err, "same unit")
}

func TestAbsentRelationshipConflictHasStableResolvedValue(t *testing.T) {
	child, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	firstParent, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{2}, 16)))
	require.NoError(t, err)
	secondParent, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{3}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 3)
	for _, values := range [][]boardhistory.Payload{
		{
			&boardhistory.IssueContainment{ChildUID: child, ParentUID: firstParent},
			&boardhistory.IssueContainment{ChildUID: child, ParentUID: secondParent},
			&boardhistory.IssueContainment{ChildUID: child, ParentUID: secondParent, Present: true},
		},
		{
			&boardhistory.ExternalKey{Key: "tracker/key", IssueUID: firstParent},
			&boardhistory.ExternalKey{Key: "tracker/key", IssueUID: secondParent},
			&boardhistory.ExternalKey{Key: "tracker/key", IssueUID: secondParent, Present: true},
		},
	} {
		candidates := []Candidate{
			{Origins: []boardhistory.EventID{origins[0]}, Value: values[0]},
			{Origins: []boardhistory.EventID{origins[1]}, Value: values[1]},
			{Origins: []boardhistory.EventID{origins[2]}, Value: values[2]},
		}
		forward, err := NewConflict("board-test", candidateSequence(candidates))
		require.NoError(t, err)
		slices.Reverse(candidates)
		reverse, err := NewConflict("board-test", candidateSequence(candidates))
		require.NoError(t, err)
		assert.Equal(t, forward.ID(), reverse.ID())
		assert.Equal(t, forward.Alternatives(), reverse.Alternatives())
		for _, alternative := range forward.Alternatives() {
			if len(alternative.Origins) != 2 {
				continue
			}
			resolved, err := forward.Resolve(ResolveRequest{ConflictID: forward.ID(), Choice: TakeAlternative{ID: alternative.ID}}, valueForAlternative(t, candidates, alternative), time.Unix(4, 0).UTC())
			require.NoError(t, err)
			switch value := resolved.Payload.(type) {
			case *boardhistory.IssueContainment:
				assert.False(t, value.Present)
			case *boardhistory.ExternalKey:
				assert.False(t, value.Present)
			default:
				t.Fatalf("unexpected assignment %T", value)
			}
		}
	}
}

func TestExternalKeyConflictAffectsOnlyPossibleOwners(t *testing.T) {
	previous, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	possible, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{2}, 16)))
	require.NoError(t, err)
	origins := concurrentOrigins(t, 2)
	conflict, err := NewConflict("board-test", candidateSequence([]Candidate{
		{Origins: []boardhistory.EventID{origins[0]}, Value: &boardhistory.ExternalKey{Key: "tracker/key", IssueUID: previous}},
		{Origins: []boardhistory.EventID{origins[1]}, Value: &boardhistory.ExternalKey{Key: "tracker/key", IssueUID: possible, Present: true}},
	}))
	require.NoError(t, err)
	assert.Equal(t, []AffectedIssue{{UID: possible, Field: issue.FieldExternalKeys}}, conflict.AffectedIssues())
}

func concurrentOrigins(t *testing.T, count int) []boardhistory.EventID {
	t.Helper()
	origins := make([]boardhistory.EventID, count)
	for i := range origins {
		writer, err := board.GenerateWriterUID(bytes.NewReader(bytes.Repeat([]byte{byte(i + 1)}, 16)))
		require.NoError(t, err)
		origins[i] = boardhistory.EventID{Writer: writer, Sequence: 1}
	}
	return origins
}

func candidateSequence(values []Candidate) iter.Seq2[Candidate, error] {
	return func(yield func(Candidate, error) bool) {
		for _, value := range values {
			if !yield(value, nil) {
				return
			}
		}
	}
}

func valueForAlternative(t *testing.T, candidates []Candidate, alternative Alternative) boardhistory.Payload {
	t.Helper()
	for _, candidate := range candidates {
		if slices.Contains(candidate.Origins, alternative.Origins[0]) {
			return candidate.Value
		}
	}
	t.Fatal("alternative has no retained representative")
	return nil
}
