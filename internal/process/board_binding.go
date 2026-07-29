package process

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.abhg.dev/cardamom/internal/board"
	"go.abhg.dev/cardamom/internal/board/selection"
)

var _ selection.Binding = (*checkoutBoardBinding)(nil)

// checkoutBoardBinding stores one board identity at a checkout-private path.
type checkoutBoardBinding struct {
	// path is selected from the process working directory during composition.
	path string
}

// Read returns the board identity bound to the checkout.
func (f *checkoutBoardBinding) Read() (board.ID, error) {
	body, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", selection.ErrBindingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read board binding %q: %w", f.path, err)
	}
	value := strings.TrimSpace(string(body))
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("board binding %q must contain one board ID", f.path)
	}
	id, err := board.NewID(value)
	if err != nil {
		return "", fmt.Errorf("parse board binding %q: %w", f.path, err)
	}
	return id, nil
}

// Write atomically replaces the board identity bound to the checkout.
func (f *checkoutBoardBinding) Write(id board.ID) error {
	temporary, err := os.CreateTemp(filepath.Dir(f.path), ".cardamom-board-*")
	if err != nil {
		return fmt.Errorf("create board binding beside %q: %w", f.path, err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set board binding permissions: %w", err)
	}
	if _, err := fmt.Fprintln(temporary, id.String()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write board binding: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close board binding: %w", err)
	}
	if err := os.Rename(temporaryPath, f.path); err != nil {
		return fmt.Errorf("replace board binding %q: %w", f.path, err)
	}
	return nil
}
