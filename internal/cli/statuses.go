package cli

import (
	"fmt"

	"github.com/rovak/beadsv2/internal/store"
)

type StatusesCmd struct{}

func (c *StatusesCmd) Run(r *runCtx) error {
	if r.json {
		return r.emitJSON(store.ValidStatuses)
	}
	for _, s := range store.ValidStatuses {
		fmt.Fprintln(r.stdout, s)
	}
	return nil
}

type TypesCmd struct{}

func (c *TypesCmd) Run(r *runCtx) error {
	if r.json {
		return r.emitJSON(store.ValidTypes)
	}
	for _, t := range store.ValidTypes {
		fmt.Fprintln(r.stdout, t)
	}
	return nil
}
