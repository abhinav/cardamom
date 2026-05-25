package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rovak/beadsv2/internal/store"
)

type StatsCmd struct{}

func (c *StatsCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		st, err := s.Stats(r.ctx)
		if err != nil {
			return err
		}
		if r.json {
			return json.NewEncoder(r.stdout).Encode(st)
		}
		printGroup(r, "Status", st.Status)
		fmt.Fprintln(r.stdout)
		printGroup(r, "Agents", st.Agents)
		fmt.Fprintln(r.stdout)
		printGroup(r, "Types", st.Types)
		return nil
	})
}

func printGroup(r *runCtx, title string, m map[string]int) {
	fmt.Fprintf(r.stdout, "%s:\n", title)
	if len(m) == 0 {
		fmt.Fprintln(r.stdout, "  (none)")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	width := 0
	for _, k := range keys {
		if len(k) > width {
			width = len(k)
		}
	}
	for _, k := range keys {
		fmt.Fprintf(r.stdout, "  %-*s  %d\n", width, k, m[k])
	}
}
