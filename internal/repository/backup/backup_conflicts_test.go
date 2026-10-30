package backup

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainbackup "go.abhg.dev/cardamom/internal/backup"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestCaptureRejectsEveryConflictCategoryBeforeOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		kind     boardhistory.EventKind
		category string
	}{
		{name: "optional Summary", kind: boardhistory.EventIssueSummarySet, category: "field"},
		{name: "board override", kind: boardhistory.EventBoardSummaryLimitSet, category: "field"},
		{name: "relationships", category: "relationships"},
		{name: "execution", category: "execution"},
	} {
		t.Run(test.name, func(t *testing.T) {
			persistence := openBackupTestStore(t, filepath.Join(t.TempDir(), "board.sqlite3"))
			seedBackupTestBoards(t, persistence)
			change, err := persistence.Change(t.Context())
			require.NoError(t, err)
			_, err = change.ExecContext(t.Context(), `INSERT INTO board_conflicts(id,board_id,kind,category,revision) VALUES('unresolved','board-two',?,?,1)`, string(test.kind), test.category)
			require.NoError(t, err)
			require.NoError(t, change.Commit())
			require.NoError(t, change.Done())
			for _, scope := range []struct {
				name      string
				selection domainbackup.Selection
			}{
				{name: "all", selection: domainbackup.AllBoards()},
				{name: "selected", selection: mustBackupSelection(t, "board-one", "board-two")},
			} {
				t.Run(scope.name, func(t *testing.T) {
					destination := &backupTestCaptureDestination{}
					_, err := New(persistence).Capture(t.Context(), scope.selection, configuration.Overrides{}, destination)
					require.ErrorContains(t, err, "unresolved")
					assert.Empty(t, destination.projects)
					assert.Empty(t, destination.boards)
				})
			}
			destination := &backupTestCaptureDestination{}
			_, err = New(persistence).Capture(t.Context(), mustBackupSelection(t, "board-one"), configuration.Overrides{}, destination)
			require.NoError(t, err)
			assert.Len(t, destination.boards, 1)
		})
	}
}
