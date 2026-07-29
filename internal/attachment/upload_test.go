package attachment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/configuration"
)

func TestUpload_Validate(t *testing.T) {
	attachment := validAttachment(t)
	upload := Upload{
		ID:               mustUploadID(t, "upload_1"),
		Association:      attachment.Association,
		Filename:         attachment.Filename,
		Actor:            "captain",
		State:            UploadStateActive,
		AcceptedOffset:   42,
		MaximumSizeBytes: configuration.ByteLimit(MaxAttachmentSizeBytes),
		ExpiresAt:        time.Unix(100, 0).UTC(),
	}
	assert.NoError(t, upload.Validate())

	t.Run("OffsetExceedsExpectedSize", func(t *testing.T) {
		value := upload
		expected := uint64(41)
		value.ExpectedSizeBytes = &expected
		assert.Error(t, value.Validate())
	})

	t.Run("CommittedWithoutAttachment", func(t *testing.T) {
		value := upload
		value.State = UploadStateCommitted
		assert.Error(t, value.Validate())
	})

	t.Run("Committed", func(t *testing.T) {
		value := upload
		value.State = UploadStateCommitted
		value.Attachment = &attachment
		assert.NoError(t, value.Validate())
	})
}

func TestWriteChunkRequest_Validate(t *testing.T) {
	request := WriteChunkRequest{
		Invocation:     NewInvocation("captain"),
		UploadID:       mustUploadID(t, "upload_1"),
		ExpectedOffset: 0,
		Content:        []byte("payload"),
	}
	assert.NoError(t, request.Validate())

	t.Run("Empty", func(t *testing.T) {
		value := request
		value.Content = nil
		assert.Error(t, value.Validate())
	})

	t.Run("TooLarge", func(t *testing.T) {
		value := request
		value.Content = make([]byte, MaxChunkSizeBytes+1)
		assert.Error(t, value.Validate())
	})
}

func mustUploadID(t *testing.T, value string) UploadID {
	t.Helper()

	id, err := NewUploadID(value)
	require.NoError(t, err)
	return id
}
