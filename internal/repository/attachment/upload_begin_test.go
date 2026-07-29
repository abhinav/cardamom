package attachment

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/errkind"
	"go.abhg.dev/cardamom/internal/issue"
	"go.abhg.dev/cardamom/internal/repository/store"
)

func TestRepositoryBeginAndStatusUpload(t *testing.T) {
	directory := t.TempDir()
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(directory, "cardamom.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at)
		VALUES ('project-test', 'Test project', 1700000000)
	`)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-test', 'project-test', 'Test board', 1700000000)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	repository, err := New(persistence, Config{
		StoreDirectory: directory,
		Clock:          &fixedClock{now: time.Unix(1_700_000_100, 0).UTC()},
		Entropy:        bytes.NewReader(bytes.Repeat([]byte{1}, 64)),
	})
	require.NoError(t, err)
	service := domainattachment.NewService(domainattachment.ServiceConfig{Repository: repository})

	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := domainattachment.NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := domainattachment.NewFilename("report.txt")
	require.NoError(t, err)

	begun, err := service.BeginUpload(t.Context(), domainattachment.BeginUploadRequest{
		Invocation:  domainattachment.NewInvocation("captain"),
		Association: association,
		Filename:    filename,
	})
	require.NoError(t, err)
	assert.Equal(t, domainattachment.UploadStateActive, begun.State)
	assert.Zero(t, begun.AcceptedOffset)
	assert.Equal(t, time.Unix(1_700_086_500, 0).UTC(), begun.ExpiresAt)

	entries, err := filepath.Glob(filepath.Join(directory, "blobs", "staging", "*"))
	require.NoError(t, err)
	assert.Len(t, entries, 1)

	status, err := service.GetUpload(t.Context(), domainattachment.GetUploadRequest{
		UploadID: begun.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, begun, status)
}

func TestRepositoryBeginRequiresDurableStaging(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{9}, 64))
	fixture.repository.blobs.syncDirectory = func(string) error {
		return assert.AnError
	}

	_, err := fixture.service.BeginUpload(
		t.Context(),
		domainattachment.BeginUploadRequest{
			Invocation:  domainattachment.NewInvocation("captain"),
			Association: fixture.association,
			Filename:    fixture.filename,
		},
	)
	assert.ErrorIs(t, err, assert.AnError)
	assert.Zero(t, countRows(t, fixture.persistence, "attachment_uploads"))
}

func TestRepositoryBeginClassifiesMissingTarget(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{13}, 128))

	t.Run("Board", func(t *testing.T) {
		boardID, err := board.NewID("missing-board")
		require.NoError(t, err)
		association, err := domainattachment.NewBoardAssociation(boardID)
		require.NoError(t, err)
		_, err = fixture.service.BeginUpload(
			t.Context(),
			domainattachment.BeginUploadRequest{
				Invocation:  domainattachment.NewInvocation("captain"),
				Association: association,
				Filename:    fixture.filename,
			},
		)
		assert.ErrorIs(t, err, domainattachment.ErrAttachmentTargetNotFound)
		assert.Equal(t, errkind.NotFound, errkind.Of(err))
	})

	t.Run("OriginIssue", func(t *testing.T) {
		issueID, err := issue.NewID("missing-issue")
		require.NoError(t, err)
		association, err := domainattachment.NewIssueAssociation(
			fixture.association.BoardID(),
			issueID,
		)
		require.NoError(t, err)
		_, err = fixture.service.BeginUpload(
			t.Context(),
			domainattachment.BeginUploadRequest{
				Invocation:  domainattachment.NewInvocation("captain"),
				Association: association,
				Filename:    fixture.filename,
			},
		)
		assert.ErrorIs(t, err, domainattachment.ErrAttachmentTargetNotFound)
		assert.Equal(t, errkind.NotFound, errkind.Of(err))
	})

	assert.Zero(t, countRows(t, fixture.persistence, "attachment_uploads"))
	staging, err := filepath.Glob(filepath.Join(
		fixture.directory,
		"blobs",
		"staging",
		"*",
	))
	require.NoError(t, err)
	assert.Empty(t, staging)
}

func TestServiceClassifiesUploadErrors(t *testing.T) {
	fixture := openUploadFixture(t, bytes.Repeat([]byte{10}, 128))
	_, err := fixture.service.GetUpload(
		t.Context(),
		domainattachment.GetUploadRequest{
			UploadID: domainattachment.UploadID("invalid upload"),
		},
	)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))

	missingID, err := domainattachment.NewUploadID("missing")
	require.NoError(t, err)
	_, err = fixture.service.GetUpload(
		t.Context(),
		domainattachment.GetUploadRequest{UploadID: missingID},
	)
	assert.Equal(t, errkind.NotFound, errkind.Of(err))
	assert.ErrorIs(t, err, domainattachment.ErrUploadNotFound)

	upload := fixture.begin(t, "captain", nil, nil)
	_, err = fixture.service.WriteChunk(t.Context(), domainattachment.WriteChunkRequest{
		Invocation: domainattachment.NewInvocation("commander"), UploadID: upload.ID,
		ExpectedOffset: 0, Content: []byte("conflict"),
	})
	assert.Equal(t, errkind.Conflict, errkind.Of(err))
	assert.ErrorIs(t, err, domainattachment.ErrUploadActorConflict)
}

type uploadFixture struct {
	directory   string
	persistence *store.Store
	repository  *Repository
	service     *domainattachment.Service
	clock       *fixedClock
	association domainattachment.Association
	filename    domainattachment.Filename
}

func openUploadFixture(t *testing.T, entropy []byte) *uploadFixture {
	t.Helper()
	directory := t.TempDir()
	persistence, err := store.Open(t.Context(), store.Config{
		Path: filepath.Join(directory, "cardamom.sqlite3"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, persistence.Close()) })

	change, err := persistence.Change(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, change.Done()) }()
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO projects (id, name, created_at)
		VALUES ('project-test', 'Test project', 1700000000)
	`)
	require.NoError(t, err)
	_, err = change.ExecContext(t.Context(), `
		INSERT INTO boards (id, project_id, name, created_at)
		VALUES ('board-test', 'project-test', 'Test board', 1700000000)
	`)
	require.NoError(t, err)
	require.NoError(t, change.Commit())

	clock := &fixedClock{now: time.Unix(1_700_000_100, 0).UTC()}
	repository, err := New(persistence, Config{
		StoreDirectory: directory,
		Clock:          clock,
		Entropy:        bytes.NewReader(entropy),
	})
	require.NoError(t, err)
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := domainattachment.NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := domainattachment.NewFilename("report.txt")
	require.NoError(t, err)
	return &uploadFixture{
		directory:   directory,
		persistence: persistence,
		repository:  repository,
		service: domainattachment.NewService(domainattachment.ServiceConfig{
			Repository: repository,
		}),
		clock:       clock,
		association: association,
		filename:    filename,
	}
}

func (f *uploadFixture) begin(
	t *testing.T,
	actor string,
	expectedSize *uint64,
	expectedDigest *domainattachment.Digest,
) domainattachment.Upload {
	t.Helper()
	upload, err := f.service.BeginUpload(t.Context(), domainattachment.BeginUploadRequest{
		Invocation:        domainattachment.NewInvocation(actor),
		Association:       f.association,
		Filename:          f.filename,
		ExpectedSizeBytes: expectedSize,
		ExpectedDigest:    expectedDigest,
	})
	require.NoError(t, err)
	return upload
}

func uploadRevision(t *testing.T, persistence *store.Store) int64 {
	t.Helper()
	revision, err := persistence.CanonicalRevision(t.Context())
	require.NoError(t, err)
	return revision
}

func attachmentCount(t *testing.T, persistence *store.Store) int {
	t.Helper()
	return countRows(t, persistence, "attachments")
}

func countRows(t *testing.T, persistence *store.Store, table string) int {
	t.Helper()
	view, err := persistence.View(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, view.Done()) }()
	var count int
	err = view.QueryRowContext(t.Context(), "SELECT count(*) FROM "+table).Scan(&count)
	require.NoError(t, err)
	return count
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time { return c.now }
