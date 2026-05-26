package cli

import (
	"errors"
	"os"

	"github.com/rovak/beadsv2/internal/store"
)

type InitCmd struct{}

func (c *InitCmd) Run(r *runCtx) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	// Detect whether the DB file already exists before letting
	// store.Open create it. Running `init` twice is safe (migrations
	// are idempotent) but the message used to lie ("initialized …")
	// when nothing changed.
	existed := true
	if _, err := os.Stat(r.dbPath()); errors.Is(err, os.ErrNotExist) {
		existed = false
	}
	s, err := store.Open(r.dbPath())
	if err != nil {
		return err
	}
	defer s.Close()
	if existed {
		r.notice("already initialized: %s\n", r.dbPath())
	} else {
		r.notice("initialized %s\n", r.dbPath())
	}
	return nil
}
