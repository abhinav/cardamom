package board

import (
	"bytes"
	"iter"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainboard "go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/boardconflict"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/issue"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	"go.abhg.dev/cardamom/internal/repository/internal/query"
	"go.abhg.dev/cardamom/internal/repository/store"
)

// conflictFixture retains real concurrent mutations while keeping preview IDs
// readable. Each contender has its own writer and observes the same baseline;
// no contender observes another contender's commit.
type conflictFixture struct {
	repository *Repository
	store      *store.Store
	entries    map[string]boardconflict.Entry
	first      map[string]string
	uids       map[string]issue.UID
	frontier   []boardhistory.Tip
	nextWriter byte
}

func seedConflictFixture(t *testing.T, directory string) *conflictFixture {
	t.Helper()
	persistence := openCopyTestStore(t, directory)
	repository, err := New(persistence, Config{BoardID: "board-conflicts", IDPrefix: "demo-", IDStrategy: "random"})
	require.NoError(t, err)
	fixture := &conflictFixture{
		repository: repository, store: persistence,
		entries: make(map[string]boardconflict.Entry), first: make(map[string]string), uids: make(map[string]issue.UID), nextWriter: 2,
	}
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
INSERT INTO projects(id, name, created_at) VALUES ('project-conflicts', 'Conflict preview', 1700000000);
INSERT INTO boards(id, project_id, name, created_at, issue_id_prefix, issue_id_strategy, revision)
VALUES ('board-conflicts', 'project-conflicts', 'Conflict repair preview', 1700000000, 'demo-', 'random', 1);
UPDATE store_state SET current_revision = 1 WHERE singleton = 1;
INSERT INTO board_replica_identities(board_id, writer_uid)
VALUES ('board-conflicts', X'01010101010101010101010101010101');`)
	require.NoError(t, err)
	for index, entry := range []struct{ id, title, kind string }{
		{"demo-title", "Concurrent title and Summary", "task"},
		{"demo-cycle-a", "Dependency cycle: first issue", "task"},
		{"demo-cycle-b", "Dependency cycle: second issue", "task"},
		{"demo-execution", "Closure versus waiting", "task"},
		{"demo-independent", "Unrelated issue remains usable", "task"},
		{"demo-checkpoint", "Concurrent checkpoint decisions", "checkpoint"},
	} {
		uid, err := issue.ParseUID(bytes.Repeat([]byte{byte(index + 1)}, 16))
		require.NoError(t, err)
		fixture.uids[entry.id] = uid
		_, err = change.ExecContext(t.Context(), `
INSERT INTO issues(uid, id, board_id, title, kind, lifecycle, priority, created_at, updated_at, summary, details, revision)
VALUES (?, ?, 'board-conflicts', ?, ?, 'open', 2, 1700000000, 1700000000,
'Original Summary', 'Known Details remain readable while other fields are disputed.', 1)`, uid.Bytes(), entry.id, entry.title, entry.kind)
		require.NoError(t, err)
	}
	require.NoError(t, repositoryhistory.InitializeV1(t.Context(), change, repository.boardID, time.Unix(1700000000, 0)))
	tips, err := query.New(change).BoardHistoryListTips(t.Context(), repository.boardID.String())
	require.NoError(t, err)
	for _, tip := range tips {
		writer, err := domainboard.ParseWriterUID(tip.WriterUid)
		require.NoError(t, err)
		hash, err := boardhistory.ParseDigest(tip.MarkerHash)
		require.NoError(t, err)
		fixture.frontier = append(fixture.frontier, boardhistory.Tip{Writer: writer, Sequence: uint64(tip.Sequence), Hash: hash})
	}
	require.NoError(t, change.Commit())
	require.NoError(t, change.Done())
	fixture.retain(t, "title", false,
		&boardhistory.IssueTitleSet{IssueUID: fixture.uids["demo-title"], Title: "Title from first replica"},
		&boardhistory.IssueTitleSet{IssueUID: fixture.uids["demo-title"], Title: "Title from second replica"})
	fixture.retain(t, "summary", false,
		&boardhistory.IssueSummarySet{IssueUID: fixture.uids["demo-title"], Summary: new("Summary from first replica")},
		&boardhistory.IssueSummarySet{IssueUID: fixture.uids["demo-title"], Summary: new("Summary from second replica")})
	fixture.retain(t, "limit", false,
		&boardhistory.BoardSummaryLimitSet{Limit: new(int64(4096))},
		&boardhistory.BoardSummaryLimitSet{Limit: new(int64(8192))})
	fixture.retain(t, "cycle", true,
		&boardhistory.IssueDependency{IssueUID: fixture.uids["demo-cycle-a"], PrerequisiteUID: fixture.uids["demo-cycle-b"], Present: true},
		&boardhistory.IssueDependency{IssueUID: fixture.uids["demo-cycle-b"], PrerequisiteUID: fixture.uids["demo-cycle-a"], Present: true})
	fixture.retain(t, "execution", true,
		&boardhistory.IssueLifecycleSet{IssueUID: fixture.uids["demo-execution"], Lifecycle: "closed", ClosedAt: new(time.Unix(1700000100, 0).UTC())},
		&boardhistory.IssueWaitingSet{IssueUID: fixture.uids["demo-execution"], Waiting: &issue.WaitingState{Reason: "Waiting for external review", Since: time.Unix(1700000100, 0).UTC()}})
	return fixture
}

func (f *conflictFixture) retain(t *testing.T, name string, constraint bool, values ...boardhistory.Payload) {
	t.Helper()
	change, err := f.store.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	queries := query.New(change)
	var firstOrigin boardhistory.EventID
	candidates := func(yield func(boardconflict.Candidate, error) bool) {
		for index, value := range values {
			candidate := f.appendConcurrentMutation(t, queries, value)[0]
			if index == 0 {
				firstOrigin = candidate.Origins[0]
			}
			if !yield(candidate, nil) {
				return
			}
		}
	}
	var entry boardconflict.Entry
	if constraint {
		entry, err = boardconflict.NewConstraintConflict(f.repository.boardID, iter.Seq2[boardconflict.Candidate, error](candidates))
	} else {
		entry, err = boardconflict.NewConflict(f.repository.boardID, iter.Seq2[boardconflict.Candidate, error](candidates))
	}
	require.NoError(t, err)
	reservation, err := change.ReserveRevision(t.Context())
	require.NoError(t, err)
	require.NoError(t, f.repository.retainConflict(t.Context(), queries, entry, reservation.Revision()))
	_, err = change.ExecContext(t.Context(), "UPDATE boards SET revision = ? WHERE id = ?", reservation.Revision(), f.repository.boardID.String())
	require.NoError(t, err)
	require.NoError(t, change.PublishRevision(t.Context(), reservation))
	require.NoError(t, change.Commit())
	f.entries[name] = entry
	switch entry := entry.(type) {
	case *boardconflict.Conflict:
		for _, alternative := range entry.Alternatives() {
			if alternative.Origins[0] == firstOrigin {
				f.first[name] = alternative.ID
			}
		}
	case *boardconflict.ConstraintConflict:
		for _, assignment := range entry.Assignments() {
			if assignment.Origins[0] == firstOrigin {
				f.first[name] = string(assignment.AlternativeID)
			}
		}
	}
	require.NotEmpty(t, f.first[name])
}

// appendConcurrentMutation retains one replica's coherent changes against the
// shared baseline. Related assignments in one mutation share a writer/sequence
// and differ by ordinal, as checkpoint decision and lifecycle changes do.
func (f *conflictFixture) appendConcurrentMutation(t *testing.T, queries *query.Queries, values ...boardhistory.Payload) []boardconflict.Candidate {
	t.Helper()
	writer, err := domainboard.ParseWriterUID(bytes.Repeat([]byte{f.nextWriter}, 16))
	require.NoError(t, err)
	f.nextWriter++
	encoder, err := boardhistory.NewMutationEncoder(boardhistory.Header{Board: f.repository.boardID, Writer: writer, Sequence: 1, Frontier: f.frontier}, func(part boardhistory.Part) error {
		return queries.BoardHistoryInsertPart(t.Context(), query.BoardHistoryInsertPartParams{BoardID: f.repository.boardID.String(), WriterUid: writer.Bytes(), Sequence: 1, PartIndex: part.Index, Bytes: part.Bytes})
	})
	require.NoError(t, err)
	candidates := make([]boardconflict.Candidate, 0, len(values))
	for ordinal, value := range values {
		require.NoError(t, encoder.AppendEvent(boardhistory.Event{OccurredAt: time.Unix(1700000100, 0), Payload: value}))
		candidates = append(candidates, boardconflict.Candidate{Origins: []boardhistory.EventID{{Writer: writer, Sequence: 1, Ordinal: uint64(ordinal)}}, Value: value})
	}
	marker, err := encoder.Finish()
	require.NoError(t, err)
	require.NoError(t, queries.BoardHistoryInsertCommit(t.Context(), query.BoardHistoryInsertCommitParams{
		BoardID: f.repository.boardID.String(), WriterUid: writer.Bytes(), Sequence: 1, PreviousHash: boardhistory.Digest{}.Bytes(), MarkerHash: marker.Hash().Bytes(), MarkerBytes: marker.Bytes(), PartCount: marker.PartCount(), EventCount: int64(marker.EventCount()), PayloadHash: marker.PayloadHash().Bytes(),
	}))
	require.NoError(t, queries.BoardHistoryInsertTip(t.Context(), query.BoardHistoryInsertTipParams{BoardID: f.repository.boardID.String(), WriterUid: writer.Bytes(), Sequence: 1, MarkerHash: marker.Hash().Bytes()}))
	return candidates
}
