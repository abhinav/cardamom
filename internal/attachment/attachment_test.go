package attachment

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/board"
)

func TestAttachment_Validate(t *testing.T) {
	attachment := validAttachment(t)
	assert.NoError(t, attachment.Validate())

	t.Run("ActiveWithRemoval", func(t *testing.T) {
		value := attachment
		value.Removed = &Attribution{
			Actor: "captain", At: time.Unix(20, 0).UTC(), Revision: 3,
		}
		assert.Error(t, value.Validate())
	})

	t.Run("RemovedWithoutAttribution", func(t *testing.T) {
		value := attachment
		value.Lifecycle = LifecycleRemoved
		assert.Error(t, value.Validate())
	})

	t.Run("Removed", func(t *testing.T) {
		value := attachment
		value.Lifecycle = LifecycleRemoved
		value.Removed = &Attribution{
			Actor: "captain", At: time.Unix(20, 0).UTC(), Revision: 3,
		}
		assert.NoError(t, value.Validate())
	})
}

func validAttachment(t *testing.T) Attachment {
	t.Helper()

	boardID, err := board.NewID("board-1")
	require.NoError(t, err)
	association, err := NewBoardAssociation(boardID)
	require.NoError(t, err)
	id, err := NewID("att_" + strings.Repeat("a", 26))
	require.NoError(t, err)
	digest, err := NewDigest("sha256:" + strings.Repeat("0", 64))
	require.NoError(t, err)
	filename, err := NewFilename("artifact.txt")
	require.NoError(t, err)
	mediaType, err := NewMediaType("text/plain; charset=utf-8")
	require.NoError(t, err)

	return Attachment{
		ID:           id,
		Association:  association,
		Blob:         BlobDescriptor{Digest: digest, SizeBytes: 42},
		Filename:     filename,
		MediaType:    mediaType,
		Lifecycle:    LifecycleActive,
		Availability: BlobAvailabilityPresentUnverified,
		Created: Attribution{
			Actor: "captain", At: time.Unix(10, 0).UTC(), Revision: 2,
		},
	}
}
