package attachment

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/configuration"
	"go.uber.org/mock/gomock"
)

func TestService_BeginUpload_resolvesLiveAdmissionLimit(t *testing.T) {
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := NewFilename("report.txt")
	require.NoError(t, err)
	first := configuration.Defaults()
	firstLimit, err := configuration.NewByteLimit(4)
	first.Attachment.MaxBytes = &firstLimit
	require.NoError(t, err)
	second := first
	secondLimit, err := configuration.NewByteLimit(2)
	second.Attachment.MaxBytes = &secondLimit
	require.NoError(t, err)
	configurations := &attachmentConfigurations{
		values: []configuration.Configuration{first, second},
	}
	repository := NewMockRepository(gomock.NewController(t))
	repository.EXPECT().BeginUpload(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, admission BeginUploadAdmission) (Upload, error) {
			assert.Equal(t, uint64(4), admission.MaximumSizeBytes.Uint64())
			uploadID, uploadErr := NewUploadID("upload-test")
			return Upload{
				ID: uploadID, Association: admission.Request.Association,
				Filename: admission.Request.Filename,
				Actor:    admission.Request.Invocation.Actor(), State: UploadStateActive,
				MaximumSizeBytes: admission.MaximumSizeBytes,
				ExpiresAt:        time.Unix(1, 0).UTC(),
			}, uploadErr
		},
	)
	service := NewService(ServiceConfig{
		Repository: repository, Configuration: configurations,
	})

	expectedSize := uint64(4)
	_, err = service.BeginUpload(t.Context(), BeginUploadRequest{
		Invocation: NewInvocation("captain"), Association: association,
		Filename: filename, ExpectedSizeBytes: &expectedSize,
	})
	require.NoError(t, err)

	expectedSize = 3
	_, err = service.BeginUpload(t.Context(), BeginUploadRequest{
		Invocation: NewInvocation("captain"), Association: association,
		Filename: filename, ExpectedSizeBytes: &expectedSize,
	})
	assert.ErrorContains(t, err, "attachment expected size 3 exceeds 2 bytes")
	assert.Equal(t, 2, configurations.calls)
}

type attachmentConfigurations struct {
	values []configuration.Configuration
	calls  int
}

func (c *attachmentConfigurations) ResolveConfiguration(
	context.Context,
	board.ID,
) (configuration.Configuration, error) {
	value := c.values[c.calls]
	c.calls++
	return value, nil
}

func TestServiceBeginUploadConsumesOnlyAttachmentConfiguration(t *testing.T) {
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := NewFilename("report.txt")
	require.NoError(t, err)
	unrelated := configuration.Defaults()
	unrelated.Issue.ID.Prefix = nil
	unrelated.Conflicts = []configuration.ConflictReference{{ID: "conflict-prefix", Field: configuration.FieldIssueIDPrefix}}
	disputed := unrelated
	disputed.Attachment.MaxBytes = nil
	disputed.Conflicts = append(disputed.Conflicts, configuration.ConflictReference{ID: "conflict-limit", Field: configuration.FieldAttachmentMaxBytes})
	repository := NewMockRepository(gomock.NewController(t))
	uploadID, err := NewUploadID("upload-test")
	require.NoError(t, err)
	repository.EXPECT().BeginUpload(gomock.Any(), gomock.Any()).Return(Upload{
		ID: uploadID, Association: association, Filename: filename,
		Actor: "captain", State: UploadStateActive, AcceptedOffset: 0,
		MaximumSizeBytes: *unrelated.Attachment.MaxBytes, ExpiresAt: time.Now().Add(time.Hour),
	}, nil)
	service := NewService(ServiceConfig{Repository: repository, Configuration: &attachmentConfigurations{values: []configuration.Configuration{unrelated, disputed}}})
	request := BeginUploadRequest{Invocation: NewInvocation("captain"), Association: association, Filename: filename}
	_, err = service.BeginUpload(t.Context(), request)
	require.NoError(t, err)
	_, err = service.BeginUpload(t.Context(), request)
	assert.ErrorContains(t, err, "conflict-limit")
}
