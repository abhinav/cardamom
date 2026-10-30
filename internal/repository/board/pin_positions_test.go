package board

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/boardcopy"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestClonePreservesPinPositions(t *testing.T) {
	for _, test := range []struct {
		name      string
		positions []int64
	}{
		{name: "gaps", positions: []int64{17, 31, 90}},
		{name: "ties", positions: []int64{17, 17, 90}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, sourceBlobs := prepareCloneSource(t)
			setPinPositions(t, source, test.positions)
			destination, destinationBlobs := prepareCloneDestination(t)
			cloner := &CloneRepository{
				Source: source, Destination: destination,
				SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
			}
			_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
			require.NoError(t, err)
			assert.Equal(t,
				cloneTableRows(t, source, "board_pins", "issue_uid,position", "position,issue_uid"),
				cloneTableRows(t, destination, "board_pins", "issue_uid,position", "position,issue_uid"))
		})
	}
}

func TestPinPagesIncludePositionTiesAndKeepBackupOrderLinear(t *testing.T) {
	source, _ := prepareCloneSource(t)
	setPinPositions(t, source, []int64{17, 17, 90})
	view, err := source.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	rows, err := view.QueryContext(t.Context(), `
SELECT issue.id FROM board_pins AS pin JOIN issues AS issue ON issue.uid = pin.issue_uid
WHERE pin.board_id = 'board-source' ORDER BY pin.position, pin.issue_uid`)
	require.NoError(t, err)
	var orderedIDs []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		orderedIDs = append(orderedIDs, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, orderedIDs, 3)
	queries := query.New(view)
	listedIDs, err := queries.BoardListPinIDs(t.Context(), "board-source")
	require.NoError(t, err)
	assert.Equal(t, orderedIDs, listedIDs)
	pager := copyRecordPager{ctx: t.Context(), queries: queries, boardID: "board-source", pageSize: 1}
	var pins []boardcopy.CopyPin
	require.True(t, pager.readPins(func(record boardcopy.Record, err error) bool {
		require.NoError(t, err)
		pins = append(pins, record.(boardcopy.CopyPin))
		return true
	}))
	require.Len(t, pins, 3)
	for index, pin := range pins {
		assert.Equal(t, orderedIDs[index], pin.IssueID)
		assert.EqualValues(t, index, pin.Order)
	}
	assert.EqualValues(t, 3, pager.counts.Pins)
}

func setPinPositions(t *testing.T, persistence *store.Store, positions []int64) {
	t.Helper()
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), "DELETE FROM board_pins WHERE board_id = 'board-source'")
	require.NoError(t, err)
	appender := repositoryhistory.NewAppender(t.Context(), change, "board-source")
	for index, id := range []string{"src-1", "src-2", "src-3"} {
		var uidBytes []byte
		require.NoError(t, change.QueryRowContext(t.Context(), "SELECT uid FROM issues WHERE id = ?", id).Scan(&uidBytes))
		uid, err := issue.ParseUID(uidBytes)
		require.NoError(t, err)
		_, err = change.ExecContext(t.Context(), `INSERT INTO board_pins(board_id, issue_uid, position) VALUES ('board-source', ?, ?)`, uidBytes, positions[index])
		require.NoError(t, err)
		require.NoError(t, appender.Append(boardhistory.Event{
			OccurredAt: time.Unix(1005, 0),
			Payload:    &boardhistory.BoardPin{IssueUID: uid, Present: true, Position: positions[index]},
		}))
	}
	require.NoError(t, appender.Finish())
	require.NoError(t, change.Commit())
}
