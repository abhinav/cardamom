package cli

import (
	"fmt"
	"strings"

	"github.com/rovak/clu/internal/store"
)

type KVCmd struct {
	Set   KVSetCmd   `cmd:"" help:"Set (or overwrite) a key."`
	Get   KVGetCmd   `cmd:"" help:"Print the value of a key."`
	Clear KVClearCmd `cmd:"" aliases:"rm,delete,unset" help:"Delete a key."`
	List  KVListCmd  `cmd:"" aliases:"ls" help:"List every key=value pair."`
}

type KVSetCmd struct {
	Key   string   `arg:"" help:"Key."`
	Value []string `arg:"" required:"" help:"Value (joined with spaces if multiple words)."`
}

func (c *KVSetCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		v := strings.Join(c.Value, " ")
		if err := s.KVSet(r.ctx, c.Key, v); err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(store.KV{Key: c.Key, Value: v})
		}
		r.notice("set %s\n", c.Key)
		return nil
	})
}

type KVGetCmd struct {
	Key string `arg:"" help:"Key."`
}

// Get prints just the value (no key, no trailing newline only what was stored
// plus a single \n) so it's safe inside shell substitution:
//
//	VAL=$(cli kv get my_key)
func (c *KVGetCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		v, err := s.KVGet(r.ctx, c.Key)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Key, err)
		}
		if r.json {
			return r.emitJSON(store.KV{Key: c.Key, Value: v})
		}
		fmt.Fprintln(r.stdout, v)
		return nil
	})
}

type KVClearCmd struct {
	Key string `arg:"" help:"Key to delete."`
}

func (c *KVClearCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		if err := s.KVDelete(r.ctx, c.Key); err != nil {
			return fmt.Errorf("%s: %w", c.Key, err)
		}
		if r.json {
			return r.emitJSON(map[string]any{"key": c.Key, "cleared": true})
		}
		r.notice("cleared %s\n", c.Key)
		return nil
	})
}

type KVListCmd struct{}

func (c *KVListCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		kvs, err := s.KVList(r.ctx)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(kvs)
		}
		if len(kvs) == 0 {
			fmt.Fprintln(r.stdout, "(empty)")
			return nil
		}
		for _, kv := range kvs {
			fmt.Fprintf(r.stdout, "%s=%s\n", kv.Key, kv.Value)
		}
		return nil
	})
}
