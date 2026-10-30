package boardconflict

import (
	"bytes"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
)

func TestOriginAssignmentsRetainGroupedExecutionAndOverridePresence(t *testing.T) {
	uid, err := issue.GenerateUID(bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
	require.NoError(t, err)
	closed := time.Unix(20, 0)
	origin := &boardhistory.IssueOrigin{UID: uid, PublicID: "cm-test", CreatedAt: closed.Add(-time.Second), UpdatedAt: closed, Title: "Title", Kind: "task", Lifecycle: "closed", ClosedAt: &closed, Priority: 2}
	values := slices.Collect(Assignments(origin))
	require.Len(t, values, 7)
	lifecycle, err := ProjectAssignment(origin, Unit{Kind: boardhistory.EventIssueLifecycleSet, IssueUID: uid})
	require.NoError(t, err)
	assert.Equal(t, &boardhistory.IssueLifecycleSet{IssueUID: uid, Lifecycle: "closed", ClosedAt: &closed}, lifecycle)
	_, err = ProjectAssignment(origin, Unit{Kind: boardhistory.EventIssueResult, IssueUID: uid})
	assert.Error(t, err)

	boardOrigin := &boardhistory.BoardOrigin{Name: "Board", CreatedAt: closed, IssueIDPrefix: new("cm-")}
	prefix, err := ProjectAssignment(boardOrigin, Unit{Kind: boardhistory.EventBoardIssueIDPrefixSet})
	require.NoError(t, err)
	assert.Equal(t, &boardhistory.BoardIssueIDPrefixSet{Prefix: new("cm-")}, prefix)
	limit, err := ProjectAssignment(boardOrigin, Unit{Kind: boardhistory.EventBoardPinLimitSet})
	require.NoError(t, err)
	assert.Equal(t, &boardhistory.BoardPinLimitSet{}, limit)
}
