package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rovak/beadsv2/internal/store"
)

type StatusesCmd struct{}

func (c *StatusesCmd) Run(r *runCtx) error {
	if r.json {
		return json.NewEncoder(r.stdout).Encode(store.ValidStatuses)
	}
	for _, s := range store.ValidStatuses {
		fmt.Fprintln(r.stdout, s)
	}
	return nil
}

type TypesCmd struct{}

func (c *TypesCmd) Run(r *runCtx) error {
	if r.json {
		return json.NewEncoder(r.stdout).Encode(store.ValidTypes)
	}
	for _, t := range store.ValidTypes {
		fmt.Fprintln(r.stdout, t)
	}
	return nil
}
