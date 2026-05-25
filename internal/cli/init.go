package cli

import (
	"fmt"
	"os"

	"github.com/rovak/beadsv2/internal/store"
)

type InitCmd struct{}

func (c *InitCmd) Run(r *runCtx) error {
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return err
	}
	s, err := store.Open(r.dbPath())
	if err != nil {
		return err
	}
	defer s.Close()
	fmt.Fprintf(r.stdout, "initialized %s\n", r.dbPath())
	return nil
}
