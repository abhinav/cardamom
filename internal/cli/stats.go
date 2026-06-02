package cli

import (
	"fmt"
	"sort"

	"github.com/arjia-labs/clu/internal/store"
)

type StatsCmd struct{}

func (c *StatsCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		st, err := s.Stats(r.ctx)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(st)
		}
		// Match `info`'s clarification: the stored 'open' bucket
		// includes the rows that `list`/`show` display as 'blocked'
		// (open + unmet deps). Without the suffix the totals look
		// inconsistent with the per-row display.
		printGroupWithLabel(r, "Status", st.Status, map[string]string{
			"open": "open (incl. blocked)",
		})
		fmt.Fprintln(r.stdout)
		printGroup(r, "Assignees", st.Assignees)
		fmt.Fprintln(r.stdout)
		printGroup(r, "Types", st.Types)
		return nil
	})
}

// printGroupWithLabel is printGroup with optional per-key relabeling.
// Used by Stats to clarify "open (incl. blocked)" in the same way info
// does, without forking the rendering logic.
func printGroupWithLabel(r *runCtx, title string, m map[string]int, relabel map[string]string) {
	display := make(map[string]int, len(m))
	for k, v := range m {
		if alt, ok := relabel[k]; ok {
			display[alt] = v
		} else {
			display[k] = v
		}
	}
	printGroup(r, title, display)
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
