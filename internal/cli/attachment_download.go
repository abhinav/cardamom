package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.abhg.dev/cardamom/internal/attachment"
	"go.abhg.dev/cardamom/internal/board"
)

type attachmentGetCommand struct {
	ID     string `arg:"" name:"id" help:"Attachment ID."`
	Output string `arg:"" name:"output" help:"Destination file path."`
	Force  bool   `name:"force" help:"Replace an existing output file instead of requiring a new path."`
}

// Help describes verified reads and create-only output behavior.
func (*attachmentGetCommand) Help() string {
	return `Write verified attachment content to a new local file. The command
verifies the complete content before writing it. An existing output file is
replaced only when --force is supplied.`
}

func (c *attachmentGetCommand) Run(
	invocation *Invocation,
	selected *board.State,
	service *attachment.Service,
) error {
	id, err := attachment.NewID(c.ID)
	if err != nil {
		return UsageErrorf("attachment get: %s", err)
	}
	opened, err := service.OpenAttachmentContent(
		invocation.Context,
		attachment.OpenContentRequest{BoardID: selected.ID(), AttachmentID: id},
	)
	if err != nil {
		return err
	}

	written, err := writeAttachmentFile(c.Output, c.Force, opened.Handle)
	if err != nil {
		return fmt.Errorf("write attachment output %q: %w", c.Output, err)
	}

	if invocation.Output.JSON() {
		return invocation.Output.WriteJSON(attachmentGetOutput{
			ID: opened.Attachment.ID.String(), Path: c.Output, Bytes: written,
		})
	}
	return invocation.Output.Noticef("wrote %s (%d bytes)", c.Output, written)
}

// writeAttachmentFile publishes a create-only destination and removes it when
// streaming or cleanup does not complete.
func writeAttachmentFile(
	path string,
	force bool,
	content io.ReadCloser,
) (int64, error) {
	if force {
		return replaceAttachmentFile(path, content)
	}

	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, errors.Join(err, content.Close())
	}
	written, copyErr := io.Copy(output, content)
	err = errors.Join(copyErr, output.Close(), content.Close())
	if err != nil {
		return written, removeFailedAttachmentFile(path, err)
	}
	return written, nil
}

// replaceAttachmentFile preserves the prior destination until a complete
// sibling temporary file is ready to rename over it.
func replaceAttachmentFile(path string, content io.ReadCloser) (int64, error) {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, errors.Join(err, content.Close())
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return 0, errors.Join(err, content.Close())
	}
	temporaryPath := temporary.Name()
	if err := temporary.Chmod(mode); err != nil {
		return 0, removeFailedAttachmentFile(
			temporaryPath,
			errors.Join(err, temporary.Close(), content.Close()),
		)
	}

	written, copyErr := io.Copy(temporary, content)
	err = errors.Join(copyErr, temporary.Close(), content.Close())
	if err != nil {
		return written, removeFailedAttachmentFile(temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return written, removeFailedAttachmentFile(temporaryPath, err)
	}
	return written, nil
}

func removeFailedAttachmentFile(path string, failure error) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return errors.Join(failure, err)
}

type attachmentGetOutput struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}
