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
	first.Attachment.MaxBytes, err = configuration.NewByteLimit(4)
	require.NoError(t, err)
	second := first
	second.Attachment.MaxBytes, err = configuration.NewByteLimit(2)
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
