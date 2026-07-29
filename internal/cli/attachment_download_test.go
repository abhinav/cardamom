package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
)

func TestAttachmentGetCommand_Run_removesPartialNewOutput(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "download.bin")
	err := runFailingAttachmentGet(t, destination, false)

	require.ErrorContains(t, err, "read attachment content")
	assert.NoFileExists(t, destination)
}

func TestAttachmentGetCommand_Run_preservesForcedOutputOnFailure(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "download.bin")
	require.NoError(t, os.WriteFile(destination, []byte("existing"), 0o600))

	err := runFailingAttachmentGet(t, destination, true)

	require.ErrorContains(t, err, "read attachment content")
	body, readErr := os.ReadFile(destination)
	require.NoError(t, readErr)
	assert.Equal(t, "existing", string(body))
}

func runFailingAttachmentGet(t *testing.T, destination string, force bool) error {
	t.Helper()
	boardID, err := board.NewID("board-test")
	require.NoError(t, err)
	selected, err := board.Load(board.Snapshot{
		ID: boardID, ProjectID: "project-test", Name: "Test",
		Created: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)
	id, err := attachment.NewID("att_aaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	association, err := attachment.NewBoardAssociation(boardID)
	require.NoError(t, err)
	filename, err := attachment.NewFilename("download.bin")
	require.NoError(t, err)
	digest, err := attachment.NewDigest(
		"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	)
	require.NoError(t, err)
	mediaType, err := attachment.NewMediaType("application/octet-stream")
	require.NoError(t, err)
	service := attachment.NewService(attachment.ServiceConfig{Repository: &failingContentRepository{
		opened: attachment.OpenedContent{
			Attachment: attachment.Attachment{
				ID: id, Association: association,
				Blob:     attachment.BlobDescriptor{Digest: digest, SizeBytes: 100},
				Filename: filename, MediaType: mediaType,
				Lifecycle:    attachment.LifecycleActive,
				Availability: attachment.BlobAvailabilityVerified,
				Created: attachment.Attribution{
					Actor: "tester", At: time.Unix(1, 0).UTC(), Revision: 1,
				},
			},
			Handle: new(failingContentHandle),
		},
	}})

	return (&attachmentGetCommand{
		ID: id.String(), Output: destination, Force: force,
	}).Run(
		&Invocation{
			Context: t.Context(), Actor: "tester",
			Output: newOutput(io.Discard, io.Discard, false, false),
			Stdin:  strings.NewReader(""),
		},
		selected,
		service,
	)
}

type failingContentRepository struct {
	attachment.Repository
	opened attachment.OpenedContent
}

func (r *failingContentRepository) OpenAttachmentContent(
	context.Context,
	attachment.OpenContentRequest,
) (attachment.OpenedContent, error) {
	return r.opened, nil
}

type failingContentHandle struct {
	read bool
}

func (h *failingContentHandle) Read(buffer []byte) (int, error) {
	if h.read {
		return 0, errors.New("read attachment content")
	}
	h.read = true
	return copy(buffer, "partial"), nil
}

func (*failingContentHandle) Seek(int64, int) (int64, error) { return 0, nil }

func (*failingContentHandle) Close() error { return nil }
