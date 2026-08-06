package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilePublisher stages one complete archive beside its destination before an
// atomic filesystem replacement.
type FilePublisher struct{}

var _ Publisher = (*FilePublisher)(nil)

// Publish writes a temporary file and replaces destination only after complete
// generation, synchronization, and close.
func (p *FilePublisher) Publish(
	ctx context.Context,
	destination string,
	write ArchiveWrite,
) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(destination) == "" {
		return errors.New("backup destination is required")
	}
	if write == nil {
		return errors.New("backup archive writer is required")
	}

	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(
		directory,
		"."+filepath.Base(destination)+"-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary backup: %w", err)
	}
	temporaryPath := temporary.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, temporary.Close())
		}
		if removeErr := os.Remove(temporaryPath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()

	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary backup: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary backup: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("replace backup %q: %w", destination, err)
	}
	return nil
}
