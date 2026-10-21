package board

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/boardclone"
	"go.abhg.dev/cardamom/internal/boardhistory"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/issue/planning"
	repositoryattachment "go.abhg.dev/cardamom/internal/repository/attachment"
	repositoryhistory "go.abhg.dev/cardamom/internal/repository/boardhistory"
	repositoryproject "go.abhg.dev/cardamom/internal/repository/project"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestCloneRepositoryPreservesSharedStateAndHistory(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{
		Source: source, Destination: destination, SourceBlobs: sourceBlobs,
		DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	out, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	assert.Equal(t, boardclone.Result{
		BoardID: "board-source", Name: "Source", ProjectID: "project-destination",
		Issues: 4, Mutations: 2, Blobs: 2,
	}, out)
	assertCloneSharedProjection(t, source, destination)
	assert.Equal(t, cloneTableRows(t, source, "board_mutation_parts", "writer_uid, sequence, part_index, bytes", "writer_uid, sequence, part_index"),
		cloneTableRows(t, destination, "board_mutation_parts", "writer_uid, sequence, part_index, bytes", "writer_uid, sequence, part_index"))
	assert.Equal(t, cloneTableRows(t, source, "board_mutation_commits", "writer_uid, sequence, marker_hash, marker_bytes", "writer_uid, sequence"),
		cloneTableRows(t, destination, "board_mutation_commits", "writer_uid, sequence, marker_hash, marker_bytes", "writer_uid, sequence"))
	assert.Equal(t, cloneTableRows(t, source, "board_mutation_blob_refs", "writer_uid, sequence, digest, size_bytes", "writer_uid, sequence, digest"),
		cloneTableRows(t, destination, "board_mutation_blob_refs", "writer_uid, sequence, digest, size_bytes", "writer_uid, sequence, digest"))
	assert.NotEqual(t, cloneTableRows(t, source, "board_replica_identities", "writer_uid", "writer_uid"),
		cloneTableRows(t, destination, "board_replica_identities", "writer_uid", "writer_uid"))
	assert.Len(t, cloneTableRows(t, source, "active_claims", "issue_uid", "issue_uid"), 1)
	assert.Empty(t, cloneTableRows(t, destination, "active_claims", "issue_uid", "issue_uid"))
	for _, descriptor := range []attachment.BlobDescriptor{copyTestBlobDescriptor(t), cloneHistoricalBlob(t)} {
		reader, err := destinationBlobs.OpenCopyBlob(t.Context(), descriptor)
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		assert.Len(t, body, int(descriptor.SizeBytes))
		assert.NoError(t, reader.Close())
	}

	_, err = cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.ErrorContains(t, err, "already contains board")
	assertCloneSharedProjection(t, source, destination)
}

func TestCloneRepositoryStartsIndependentWriterChains(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	_, _, err := repositoryproject.New(source, repositoryproject.Config{}).UnarchiveBoard(t.Context(), "board-source")
	require.NoError(t, err)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err = cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	before := readBoardHistory(t, destination, "board-source")
	require.Len(t, before, 3)
	cloneRepository, err := New(destination, Config{BoardID: "board-source", IDPrefix: "clone-", IDStrategy: "random"})
	require.NoError(t, err)
	planner := planning.NewPlanner(cloneRepository, cloneRepository, nil)
	for _, title := range []string{"First local edit", "Second local edit"} {
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
			Title: title, Type: "task", Priority: 2,
		})
		require.NoError(t, err)
	}
	mutations := readBoardHistory(t, destination, "board-source")
	require.Len(t, mutations, 5)
	var local []retainedHistoryMutation
	for _, mutation := range mutations {
		if mutation.marker.Header().Writer != before[0].marker.Header().Writer {
			local = append(local, mutation)
		}
	}
	require.Len(t, local, 2)
	assert.Equal(t, uint64(1), local[0].marker.Header().Sequence)
	assert.Equal(t, boardhistory.Digest{}, local[0].marker.Header().Previous)
	require.Len(t, local[0].marker.Header().Frontier, 1)
	assert.Equal(t, before[2].marker.Hash(), local[0].marker.Header().Frontier[0].Hash)
	assert.Equal(t, uint64(2), local[1].marker.Header().Sequence)
	assert.Equal(t, local[0].marker.Hash(), local[1].marker.Header().Previous)
	assert.Len(t, local[1].marker.Header().Frontier, 2)
	assert.Len(t, readBoardHistory(t, source, "board-source"), 3)

	third, thirdBlobs := prepareCloneDestination(t)
	secondClone := &CloneRepository{
		Source: destination, Destination: third,
		SourceBlobs: destinationBlobs, DestinationBlobs: thirdBlobs, Entropy: rand.Reader,
	}
	_, err = secondClone.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	assertCloneSharedProjection(t, destination, third)
	thirdRepository, err := New(third, Config{BoardID: "board-source", IDPrefix: "third-", IDStrategy: "random"})
	require.NoError(t, err)
	_, err = planning.NewPlanner(thirdRepository, thirdRepository, nil).CreateIssue(t.Context(),
		issue.NewInvocation("captain"), planning.CreateIssueRequest{Title: "Third writer", Type: "task", Priority: 2})
	require.NoError(t, err)
	thirdHistory := readBoardHistory(t, third, "board-source")
	require.Len(t, thirdHistory, 6)
	for _, mutation := range thirdHistory {
		header := mutation.marker.Header()
		if header.Writer != before[0].marker.Header().Writer && header.Writer != local[0].marker.Header().Writer {
			assert.Equal(t, uint64(1), header.Sequence)
			assert.Len(t, header.Frontier, 2)
		}
	}
}

func TestCloneRepositoryClonesReplicaBeforeFirstEdit(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	third, thirdBlobs := prepareCloneDestination(t)
	next := &CloneRepository{
		Source: destination, Destination: third,
		SourceBlobs: destinationBlobs, DestinationBlobs: thirdBlobs, Entropy: rand.Reader,
	}
	out, err := next.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	assert.Equal(t, 2, out.Mutations)
	assertCloneSharedProjection(t, source, third)
	assert.NotEqual(t, cloneTableRows(t, destination, "board_replica_identities", "writer_uid", "writer_uid"),
		cloneTableRows(t, third, "board_replica_identities", "writer_uid", "writer_uid"))
}

func TestCloneRepositoryRollsBackMissingHistoricalBlob(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs:      &missingCloneBlob{source: sourceBlobs, missing: cloneHistoricalBlob(t)},
		DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.ErrorContains(t, err, "historical blob unavailable")
	view, err := destination.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var count int
	require.NoError(t, view.QueryRowContext(t.Context(), "SELECT count(*) FROM boards").Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, view.QueryRowContext(t.Context(), "SELECT current_revision FROM store_state").Scan(&count))
	assert.Zero(t, count)
}

func TestCloneRepositoryPreservesPagedProjectionAndMultipartHistory(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	_, _, err := repositoryproject.New(source, repositoryproject.Config{}).UnarchiveBoard(t.Context(), "board-source")
	require.NoError(t, err)
	repository, err := New(source, Config{BoardID: "board-source", IDPrefix: "page-", IDStrategy: "random"})
	require.NoError(t, err)
	planner := planning.NewPlanner(repository, repository, nil)
	for index := range 130 {
		details := "Small body"
		if index == 0 {
			details = strings.Repeat("large body\n", 8000)
		}
		_, err := planner.CreateIssue(t.Context(), issue.NewInvocation("captain"), planning.CreateIssueRequest{
			Title: fmt.Sprintf("Paged issue %d", index), Type: "task", Priority: 2, Details: details,
		})
		require.NoError(t, err)
	}
	destination, destinationBlobs := prepareCloneDestination(t)
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	out, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.NoError(t, err)
	assert.Equal(t, 134, out.Issues)
	assert.Equal(t, 133, out.Mutations)
	assertCloneSharedProjection(t, source, destination)
	assert.Equal(t, cloneTableRows(t, source, "board_mutation_parts", "writer_uid,sequence,part_index,bytes", "writer_uid,sequence,part_index"),
		cloneTableRows(t, destination, "board_mutation_parts", "writer_uid,sequence,part_index,bytes", "writer_uid,sequence,part_index"))
}

func TestCloneRepositoryRejectsIdentityCollisions(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	directory := t.TempDir()
	destination := openCopyTestStore(t, directory)
	seedCopyDestinationCollisions(t, destination)
	destinationBlobs := openCopyAttachmentRepository(t, destination, directory)
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.ErrorContains(t, err, `collides with source issue "src-1"`)
	assert.Empty(t, cloneTableRows(t, destination, "issues", "id", "id"))
}

func TestCloneRepositoryRejectsPrivateUIDCollision(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	directory := t.TempDir()
	destination := openCopyTestStore(t, directory)
	seedCopyDestinationCollisions(t, destination)
	destinationBlobs := openCopyAttachmentRepository(t, destination, directory)
	view, err := source.View(t.Context())
	require.NoError(t, err)
	var uid []byte
	require.NoError(t, view.QueryRowContext(t.Context(), "SELECT uid FROM issues WHERE id='src-1'").Scan(&uid))
	require.NoError(t, view.Done())
	change, err := destination.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), "DELETE FROM issue_log_entries; DELETE FROM issues")
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `INSERT INTO issues
    (uid, id, board_id, title, kind, lifecycle, priority, created_at, updated_at, revision)
    VALUES (?, 'unrelated', 'board-existing', 'Other issue', 'task', 'open', 2, 1, 1, 1)`, uid)
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err = cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.ErrorContains(t, err, `collides with source issue "src-1"`)
	assert.Empty(t, cloneTableRows(t, destination, "issues", "id", "id"))
}

func TestCloneRepositoryRejectsLogIdentityCollision(t *testing.T) {
	source, sourceBlobs := prepareCloneSource(t)
	directory := t.TempDir()
	destination := openCopyTestStore(t, directory)
	seedCopyDestinationCollisions(t, destination)
	destinationBlobs := openCopyAttachmentRepository(t, destination, directory)
	change, err := destination.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), "UPDATE issues SET id='unrelated'")
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	cloner := &CloneRepository{
		Source: source, Destination: destination,
		SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
	}
	_, err = cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
	require.ErrorContains(t, err, `destination Log identity`)
	assert.Empty(t, cloneTableRows(t, destination, "issues", "id", "id"))
}

func TestCloneRepositoryRejectsSequentialConfiguration(t *testing.T) {
	for _, side := range []string{"source", "destination"} {
		t.Run(strings.ToUpper(side[:1])+side[1:], func(t *testing.T) {
			source, sourceBlobs := prepareCloneSource(t)
			destination, destinationBlobs := prepareCloneDestination(t)
			cloner := &CloneRepository{
				Source: source, Destination: destination,
				SourceBlobs: sourceBlobs, DestinationBlobs: destinationBlobs, Entropy: rand.Reader,
			}
			strategy := configuration.IDStrategySequential
			if side == "source" {
				change, err := source.Change(t.Context())
				require.NoError(t, err)
				_, err = change.ExecContext(t.Context(), "UPDATE projects SET issue_id_strategy='sequential'")
				require.NoError(t, err)
				require.NoError(t, change.Commit())
			} else {
				cloner.DestinationConfiguration.Issue.ID.Strategy = &strategy
			}
			_, err := cloner.Clone(t.Context(), boardclone.Request{BoardID: "board-source", ProjectID: "project-destination"})
			require.ErrorContains(t, err, side+" board uses sequential")
		})
	}
}

// prepareCloneSource supplies all shared projection families, active local
// custody, an archived board, nullable overrides, and a historical-only blob.
func prepareCloneSource(t *testing.T) (*store.Store, *repositoryattachment.Repository) {
	t.Helper()
	directory := t.TempDir()
	persistence := openCopyTestStore(t, directory)
	blobs := openCopyAttachmentRepository(t, persistence, directory)
	require.NoError(t, blobs.PublishCopyBlob(t.Context(), copyTestBlobDescriptor(t), strings.NewReader("data")))
	require.NoError(t, blobs.PublishCopyBlob(t.Context(), cloneHistoricalBlob(t), strings.NewReader("historical")))
	seedCopySource(t, persistence, copyTestBlobDescriptor(t))
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
UPDATE projects SET issue_id_strategy='random';
UPDATE boards SET issue_id_prefix='clone-', issue_summary_max_bytes=5000,
    archived_at=1002, archived_by='captain', archive_reason='Completed';
INSERT INTO active_claims (issue_uid, board_id, actor, started_at, started_revision)
VALUES ((SELECT uid FROM issues WHERE id='src-2'), 'board-source', 'worker', 1002, 2);`)
	require.NoError(t, err)
	require.NoError(t, repositoryhistory.InitializeV1(t.Context(), change, "board-source", time.Unix(1003, 0)))
	historical := cloneHistoricalBlob(t)
	_, err = change.ExecContext(t.Context(), "INSERT INTO attachment_blobs (digest, size_bytes) VALUES (?, ?)", historical.Digest.String(), historical.SizeBytes)
	require.NoError(t, err)
	appender := repositoryhistory.NewAppender(t.Context(), change, "board-source")
	require.NoError(t, appender.Append(boardhistory.Event{
		OccurredAt: time.Unix(1004, 0), Payload: &boardhistory.AttachmentAdd{
			ID: "att_aaaaaaaaaaaaaaaaaaaaaaaaaa", Digest: historical.Digest.String(), SizeBytes: int64(historical.SizeBytes),
			Filename: "old.txt", MediaType: "text/plain", Actor: "worker", At: time.Unix(1004, 0),
		},
	}))
	require.NoError(t, appender.Finish())
	require.NoError(t, change.Commit())
	return persistence, blobs
}

func prepareCloneDestination(t *testing.T) (*store.Store, *repositoryattachment.Repository) {
	t.Helper()
	directory := t.TempDir()
	persistence := openCopyTestStore(t, directory)
	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), "INSERT INTO projects (id, name, created_at) VALUES ('project-destination', 'Destination', 1000)")
	require.NoError(t, err)
	require.NoError(t, change.Commit())
	return persistence, openCopyAttachmentRepository(t, persistence, directory)
}

func cloneHistoricalBlob(t *testing.T) attachment.BlobDescriptor {
	t.Helper()
	digest, err := attachment.NewDigest(fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("historical"))))
	require.NoError(t, err)
	return attachment.BlobDescriptor{Digest: digest, SizeBytes: 10}
}

func assertCloneSharedProjection(t *testing.T, source, destination *store.Store) {
	t.Helper()
	for _, table := range []struct{ name, columns, order string }{
		{"issues", "uid,id,title,kind,lifecycle,priority,created_at,updated_at,closed_at,waiting_reason,waiting_since,summary,details", "uid"},
		{"issue_labels", "issue_uid,label", "issue_uid,label"},
		{"dependencies", "issue_uid,prerequisite_uid", "issue_uid,prerequisite_uid"},
		{"containment", "child_uid,parent_uid", "child_uid"},
		{"issue_external_keys", "external_key,issue_uid", "external_key"},
		{"issue_log_entries", "id,issue_uid,kind,author,committer,body,created_at,next_action", "local_sequence"},
		{"issue_states", "issue_uid,body,author,updated_at,snapshot_log_entry_id,next_action", "issue_uid"},
		{"issue_results", "issue_uid,body", "issue_uid"},
		{"checkpoint_decisions", "issue_uid,outcome,reason,decided_at", "issue_uid"},
		{"board_pins", "issue_uid", "position"},
		{"attachments", "id,origin_issue_uid,blob_digest,blob_size_bytes,filename,media_type,lifecycle,created_actor,created_at,removed_actor,removed_at", "id"},
	} {
		assert.Equal(t, cloneTableRows(t, source, table.name, table.columns, table.order),
			cloneTableRows(t, destination, table.name, table.columns, table.order), table.name)
	}
	columns := "id,name,description,created_at,archived_at,archived_by,archive_reason,issue_id_prefix,issue_id_strategy,issue_summary_max_bytes,attachment_max_bytes,board_pins_max_count"
	assert.Equal(t, cloneTableRows(t, source, "boards", columns, "id"), cloneTableRows(t, destination, "boards", columns, "id"))
}

// cloneTableRows compares the persisted shared contract independently of the
// production projection reader. Table and column selections are test constants.
func cloneTableRows(t *testing.T, persistence *store.Store, table, columns, order string) [][]any {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	key := "board_id"
	if table == "boards" {
		key = "id"
	}
	rows, err := view.QueryContext(t.Context(), "SELECT "+columns+" FROM "+table+" WHERE "+key+"='board-source' ORDER BY "+order)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()
	var result [][]any
	for rows.Next() {
		values := make([]any, len(strings.Split(columns, ",")))
		pointers := make([]any, len(values))
		for index := range values {
			pointers[index] = &values[index]
		}
		require.NoError(t, rows.Scan(pointers...))
		for index, value := range values {
			if data, ok := value.([]byte); ok {
				values[index] = bytes.Clone(data)
			}
		}
		result = append(result, values)
	}
	require.NoError(t, rows.Err())
	return result
}

type missingCloneBlob struct {
	source  CloneBlobSource
	missing attachment.BlobDescriptor
}

func (b *missingCloneBlob) OpenCopyBlob(ctx context.Context, descriptor attachment.BlobDescriptor) (io.ReadCloser, error) {
	if descriptor == b.missing {
		return nil, errors.New("historical blob unavailable")
	}
	return b.source.OpenCopyBlob(ctx, descriptor)
}
