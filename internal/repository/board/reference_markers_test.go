package board

import (
	"bytes"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestReferenceMarkerFollowsLocalUIDAndRetainsAbsentCandidate(t *testing.T) {
	source, _ := prepareCloneSource(t)
	marker := retireCloneSourceReference(t, source)
	view, err := NewLocator(source).ReadMarker(t.Context(), marker.ID)
	require.NoError(t, err)
	require.NotNil(t, view)
	require.Len(t, view.Candidates, 2)
	assert.Equal(t, marker.Candidates[0].ID, view.Candidates[0].ID)
	assert.True(t, view.Candidates[0].Available)
	assert.NotNil(t, view.Candidates[0].Title)
	assert.False(t, view.Candidates[1].Available)
	assert.Nil(t, view.Candidates[1].Title)
	references, err := (&Repository{store: source, boardID: "board-source"}).ResolveIssueReferences(
		t.Context(), []issue.ID{marker.ID, marker.Candidates[0].ID},
	)
	require.NoError(t, err)
	assert.ElementsMatch(t, []issue.ID{marker.ID, marker.Candidates[0].ID}, references)

	change, err := source.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), "UPDATE issues SET id='later-public-id' WHERE uid=?", view.Candidates[0].UID.Bytes())
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	view, err = NewLocator(source).ReadMarker(t.Context(), marker.ID)
	require.NoError(t, err)
	assert.Equal(t, issue.ID("later-public-id"), view.Candidates[0].ID)
	_, err = NewLocator(source).BoardForIssue(t.Context(), marker.ID.String())
	assert.ErrorContains(t, err, "ambiguous")
}

func TestClonePreservesReferenceMarkersWithoutOtherBoard(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	marker := retireCloneSourceReference(t, source)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := CloneRepository{
		Source: source, Destination: destination, SourceBlobs: sourceBlobs,
		DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	view, err := NewLocator(destination).ReadMarker(t.Context(), marker.ID)
	require.NoError(t, err)
	assert.Equal(t, marker, view)
	assert.Equal(t, readBoardHistory(t, source, "board-source"), readBoardHistory(t, destination, "board-source"))

	change, err := destination.Change(t.Context())
	require.NoError(t, err)
	// Roll back before testing cancels t.Context.
	// Otherwise database/sql can roll it back before cleanup.
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), "UPDATE issues SET id=? WHERE uid=?", marker.ID.String(), marker.Candidates[0].UID.Bytes())
	assert.ErrorContains(t, err, "retired reference")
}

func TestReferenceCandidateUnionIsIndependentOfArrivalAndGrouping(t *testing.T) {
	uids := make([]issue.UID, 3)
	for index := range uids {
		var err error
		uids[index], err = issue.ParseUID(bytes.Repeat([]byte{byte(index + 1)}, 16))
		require.NoError(t, err)
	}
	for _, order := range [][]int{{0, 1, 2}, {2, 0, 1}, {1, 2, 0}} {
		t.Run(string(rune('A'+order[0])), func(t *testing.T) {
			persistence, _ := prepareCloneDestination(t)
			for _, index := range order {
				change, err := persistence.Change(t.Context())
				require.NoError(t, err)
				queries := query.New(change)
				require.NoError(t, queries.BoardInsertReferenceMarker(t.Context(), "card-retired"))
				// Repeating the previously known participant models overlapping
				// evidence groups rather than a disjoint sequence of inserts.
				for _, candidate := range []int{order[0], index} {
					require.NoError(t, queries.BoardInsertReferenceCandidate(t.Context(), query.BoardInsertReferenceCandidateParams{
						MarkerID: "card-retired", IssueUid: uids[candidate].Bytes(), BoardID: "absent-board",
						ReplacementID: issue.ReplacementID("card-retired", uids[candidate]).String(),
					}))
				}
				require.NoError(t, change.Commit())
			}
			marker, err := NewLocator(persistence).ReadMarker(t.Context(), "card-retired")
			require.NoError(t, err)
			require.Len(t, marker.Candidates, 3)
			for index, candidate := range marker.Candidates {
				assert.Equal(t, uids[index], candidate.UID)
				assert.Equal(t, issue.ReplacementID("card-retired", uids[index]), candidate.ID)
			}
		})
	}
}

// retireCloneSourceReference creates the source state a collision merge will
// publish: one renamed local participant, one absent participant, and history
// retaining the public-ID change and collision evidence in one mutation.
func retireCloneSourceReference(t *testing.T, persistence *store.Store) *issue.ReferenceMarker {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	queries := query.New(change)
	uidBytes, err := queries.BoardGetIssueUID(t.Context(), query.BoardGetIssueUIDParams{BoardID: "board-source", ID: "src-1"})
	require.NoError(t, err)
	uid, err := issue.ParseUID(uidBytes)
	require.NoError(t, err)
	absent, err := issue.ParseUID(bytes.Repeat([]byte{255}, 16))
	require.NoError(t, err)
	retired := issue.ID("src-1")
	replacement := issue.ReplacementID(retired, uid)
	_, err = change.ExecContext(t.Context(), "UPDATE issues SET id=? WHERE uid=?", replacement.String(), uid.Bytes())
	require.NoError(t, err)
	require.NoError(t, queries.BoardInsertReferenceMarker(t.Context(), retired.String()))
	candidates := []boardhistory.ReferenceCandidate{{IssueUID: uid, BoardID: "board-source"}, {IssueUID: absent, BoardID: "board-absent"}}
	for _, candidate := range candidates {
		require.NoError(t, queries.BoardInsertReferenceCandidate(t.Context(), query.BoardInsertReferenceCandidateParams{
			MarkerID: retired.String(), IssueUid: candidate.IssueUID.Bytes(), BoardID: candidate.BoardID.String(),
			ReplacementID: issue.ReplacementID(retired, candidate.IssueUID).String(),
		}))
	}
	appender := repositoryhistory.NewAppender(t.Context(), change, "board-source")
	require.NoError(t, appender.Append(boardhistory.Event{OccurredAt: time.Unix(1005, 0), Payload: &boardhistory.IssuePublicIDSet{IssueUID: uid, PublicID: replacement.String()}}))
	require.NoError(t, appender.Append(boardhistory.Event{OccurredAt: time.Unix(1005, 0), Payload: &boardhistory.ReferenceMarker{PublicID: retired.String(), Candidates: candidates}}))
	require.NoError(t, appender.Finish())
	require.NoError(t, change.Commit())
	marker, err := NewLocator(persistence).ReadMarker(t.Context(), retired)
	require.NoError(t, err)
	return marker
}
