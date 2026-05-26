package cli

import (
	"fmt"

	"github.com/rovak/clu/internal/store"
)

type InfoCmd struct{}

type infoOut struct {
	Dir            string         `json:"dir"`
	DBPath         string         `json:"db_path"`
	IDPrefix       string         `json:"id_prefix"`
	SchemaVersion  int            `json:"schema_version"`
	IssuesTotal    int            `json:"issues_total"`
	IssuesByStatus map[string]int `json:"issues_by_status"`
}

func (c *InfoCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		v, err := s.DBVersion(r.ctx)
		if err != nil {
			return err
		}
		st, err := s.Stats(r.ctx)
		if err != nil {
			return err
		}
		total := 0
		for _, n := range st.Status {
			total += n
		}
		out := infoOut{
			Dir:            r.dir,
			DBPath:         r.dbPath(),
			IDPrefix:       s.IDPrefix(),
			SchemaVersion:  v,
			IssuesTotal:    total,
			IssuesByStatus: st.Status,
		}
		if r.json {
			return r.emitJSON(out)
		}
		fmt.Fprintf(r.stdout, "Dir:              %s\n", out.Dir)
		fmt.Fprintf(r.stdout, "DB:               %s\n", out.DBPath)
		fmt.Fprintf(r.stdout, "ID prefix:        %s\n", out.IDPrefix)
		fmt.Fprintf(r.stdout, "Schema version:   %d\n", out.SchemaVersion)
		fmt.Fprintf(r.stdout, "Total issues:     %d\n", out.IssuesTotal)
		if len(out.IssuesByStatus) > 0 {
			fmt.Fprintln(r.stdout, "By status:")
			for _, k := range store.ValidStatuses {
				if n, ok := out.IssuesByStatus[k]; ok {
					// `open` here is the stored status; `list`/`show`
					// further compute a derived "blocked" subset of
					// open. Spelling that out avoids the
					// "list shows 3 blocked but info says 1 open"
					// reconciliation confusion.
					label := k
					if k == "open" {
						label = "open (incl. blocked)"
					}
					fmt.Fprintf(r.stdout, "  %-22s  %d\n", label, n)
				}
			}
		}
		return nil
	})
}
