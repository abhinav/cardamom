package attachment

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainattachment "go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/errkind"
)

func TestRepositoryResolveAttachments(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	active := fixture.commit(t, fixture.association, "active")
	removed := fixture.commit(t, fixture.association, "removed")
	_, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation:   domainattachment.NewInvocation("captain"),
			BoardID:      fixture.association.BoardID(),
			AttachmentID: removed.ID,
		},
	)
	require.NoError(t, err)

	otherBoardID := addAttachmentBoard(t, fixture, "board-other")
	otherAssociation, err := domainattachment.NewBoardAssociation(otherBoardID)
	require.NoError(t, err)
	other := fixture.commit(t, otherAssociation, "other")
	unknown, err := domainattachment.NewID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)

	resolutions, err := fixture.service.ResolveAttachments(
		t.Context(),
		domainattachment.ResolveRequest{
			BoardID: fixture.association.BoardID(),
			AttachmentIDs: []domainattachment.ID{
				active.ID,
				removed.ID,
				other.ID,
				unknown,
				active.ID,
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, resolutions, 5)

	assert.Equal(t, active.ID, resolutions[0].AttachmentID)
	assert.Equal(t, domainattachment.ResolutionActive, resolutions[0].State)
	require.NotNil(t, resolutions[0].Attachment)
	assert.Equal(t, domainattachment.BlobAvailabilityPresentUnverified,
		resolutions[0].Attachment.Availability)

	assert.Equal(t, removed.ID, resolutions[1].AttachmentID)
	assert.Equal(t, domainattachment.ResolutionRemoved, resolutions[1].State)
	require.NotNil(t, resolutions[1].Attachment)
	assert.Equal(t, domainattachment.LifecycleRemoved,
		resolutions[1].Attachment.Lifecycle)

	for _, resolution := range resolutions[2:4] {
		assert.Equal(t, domainattachment.ResolutionUnknown, resolution.State)
		assert.Nil(t, resolution.Attachment)
	}
	assert.Equal(t, other.ID, resolutions[2].AttachmentID)
	assert.Equal(t, unknown, resolutions[3].AttachmentID)
	assert.Equal(t, resolutions[0], resolutions[4])
}

func TestRepositoryResolveAttachmentsAllowsEmptyBatch(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())

	empty, err := fixture.service.ResolveAttachments(
		t.Context(),
		domainattachment.ResolveRequest{
			BoardID: fixture.association.BoardID(),
		},
	)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestServiceResolveAttachmentsEnforcesBound(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	active := fixture.commit(t, fixture.association, "active")

	attachmentIDs := make(
		[]domainattachment.ID,
		domainattachment.MaxResolveAttachmentIDs,
	)
	for index := range attachmentIDs {
		attachmentIDs[index] = active.ID
	}
	resolutions, err := fixture.service.ResolveAttachments(
		t.Context(),
		domainattachment.ResolveRequest{
			BoardID:       fixture.association.BoardID(),
			AttachmentIDs: attachmentIDs,
		},
	)
	require.NoError(t, err)
	assert.Len(t, resolutions, domainattachment.MaxResolveAttachmentIDs)

	attachmentIDs = make(
		[]domainattachment.ID,
		domainattachment.MaxResolveAttachmentIDs+1,
	)
	for index := range attachmentIDs {
		attachmentIDs[index] = active.ID
	}
	_, err = fixture.service.ResolveAttachments(
		t.Context(),
		domainattachment.ResolveRequest{
			BoardID:       fixture.association.BoardID(),
			AttachmentIDs: attachmentIDs,
		},
	)
	assert.Equal(t, errkind.InvalidInput, errkind.Of(err))
}

func TestRepositoryOpenAttachmentContentSupportsRangeReads(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	attachment := fixture.commit(t, fixture.association, "0123456789")

	opened, err := fixture.service.OpenAttachmentContent(
		t.Context(),
		domainattachment.OpenContentRequest{
			BoardID:      fixture.association.BoardID(),
			AttachmentID: attachment.ID,
		},
	)
	require.NoError(t, err)
	assert.Equal(t, attachment.ID, opened.Attachment.ID)
	assert.Equal(t, domainattachment.BlobAvailabilityVerified,
		opened.Attachment.Availability)
	_, exposesFile := opened.Handle.(*os.File)
	assert.False(t, exposesFile)
	closed := false
	t.Cleanup(func() {
		if !closed {
			assert.NoError(t, opened.Handle.Close())
		}
	})

	offset, err := opened.Handle.Seek(4, io.SeekStart)
	require.NoError(t, err)
	assert.Equal(t, int64(4), offset)
	rangeContent := make([]byte, 3)
	_, err = io.ReadFull(opened.Handle, rangeContent)
	require.NoError(t, err)
	assert.Equal(t, "456", string(rangeContent))

	_, err = opened.Handle.Seek(0, io.SeekStart)
	require.NoError(t, err)
	content, err := io.ReadAll(opened.Handle)
	require.NoError(t, err)
	assert.Equal(t, "0123456789", string(content))

	require.NoError(t, opened.Handle.Close())
	closed = true
	_, err = opened.Handle.Read(make([]byte, 1))
	assert.Error(t, err)
}

func TestRepositoryOpenAttachmentContentClassifiesOutcomes(t *testing.T) {
	fixture := openUploadFixture(t, maintenanceEntropy())
	removed := fixture.commit(t, fixture.association, "removed")
	missing := fixture.commit(t, fixture.association, "missing")
	sizeMismatch := fixture.commit(t, fixture.association, "expected-size")
	digestMismatch := fixture.commit(t, fixture.association, "expected")

	_, err := fixture.service.RemoveAttachment(
		t.Context(),
		domainattachment.RemoveRequest{
			Invocation:   domainattachment.NewInvocation("captain"),
			BoardID:      fixture.association.BoardID(),
			AttachmentID: removed.ID,
		},
	)
	require.NoError(t, err)
	require.NoError(t, os.Remove(
		fixture.repository.blobs.contentPath(missing.Blob.Digest),
	))
	require.NoError(t, os.Truncate(
		fixture.repository.blobs.contentPath(sizeMismatch.Blob.Digest),
		1,
	))
	require.NoError(t, os.WriteFile(
		fixture.repository.blobs.contentPath(digestMismatch.Blob.Digest),
		[]byte("damaged!"),
		0o600,
	))

	unknown, err := domainattachment.NewID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	tests := []struct {
		name         string
		attachmentID domainattachment.ID
		wantError    error
		wantKind     errkind.Kind
	}{
		{
			name: "Unknown", attachmentID: unknown,
			wantError: domainattachment.ErrAttachmentNotFound,
			wantKind:  errkind.NotFound,
		},
		{
			name: "Removed", attachmentID: removed.ID,
			wantError: domainattachment.ErrAttachmentRemoved,
			wantKind:  errkind.Conflict,
		},
		{
			name: "Missing", attachmentID: missing.ID,
			wantError: domainattachment.ErrAttachmentContentMissing,
			wantKind:  errkind.NotFound,
		},
		{
			name: "SizeMismatch", attachmentID: sizeMismatch.ID,
			wantError: domainattachment.ErrAttachmentContentSizeMismatch,
			wantKind:  errkind.Conflict,
		},
		{
			name: "DigestMismatch", attachmentID: digestMismatch.ID,
			wantError: domainattachment.ErrAttachmentContentDigestMismatch,
			wantKind:  errkind.Conflict,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened, err := fixture.service.OpenAttachmentContent(
				t.Context(),
				domainattachment.OpenContentRequest{
					BoardID:      fixture.association.BoardID(),
					AttachmentID: tt.attachmentID,
				},
			)
			assert.ErrorIs(t, err, tt.wantError)
			assert.Equal(t, tt.wantKind, errkind.Of(err))
			assert.Nil(t, opened.Handle)
		})
	}
}
