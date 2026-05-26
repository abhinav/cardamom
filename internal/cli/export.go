package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rovak/clu/internal/store"
)

// exportLine is the wire format for JSONL. One per line. The "kind"
// field discriminates; "data" carries kind-specific fields.
//
// Issue:  {"kind":"issue","data":{...issue fields..., "labels":["x"]}}
// Dep:    {"kind":"dep","data":{"child":"bd-x","parent":"bd-y"}}
type exportLine struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type issueExport struct {
	store.Issue
	Labels []string `json:"labels,omitempty"`
}

type depExport struct {
	Child  string `json:"child"`
	Parent string `json:"parent"`
}

type ExportCmd struct {
	Out string `short:"o" name:"out" help:"Write JSONL to this file. Prefer this over '> file.jsonl' — -o writes only data to the file; the summary notice goes to stderr."`
}

func (c *ExportCmd) Run(r *runCtx) error {
	return withStore(r, func(s *store.Store) error {
		var w io.Writer = r.stdout
		if c.Out != "" {
			f, err := os.Create(c.Out)
			if err != nil {
				return err
			}
			defer f.Close()
			w = f
		}
		issues, err := s.List(r.ctx, store.ListFilter{})
		if err != nil {
			return err
		}
		ids := make([]string, len(issues))
		for i, is := range issues {
			ids[i] = is.ID
		}
		labelsMap, err := s.LoadLabels(r.ctx, ids)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		for _, is := range issues {
			data, err := json.Marshal(issueExport{Issue: is, Labels: labelsMap[is.ID]})
			if err != nil {
				return err
			}
			if err := enc.Encode(exportLine{Kind: "issue", Data: data}); err != nil {
				return err
			}
		}
		deps, err := s.AllDeps(r.ctx)
		if err != nil {
			return err
		}
		for _, d := range deps {
			data, err := json.Marshal(depExport{Child: d.ChildID, Parent: d.ParentID})
			if err != nil {
				return err
			}
			if err := enc.Encode(exportLine{Kind: "dep", Data: data}); err != nil {
				return err
			}
		}
		comments, err := s.AllComments(r.ctx)
		if err != nil {
			return err
		}
		for _, cm := range comments {
			data, err := json.Marshal(cm)
			if err != nil {
				return err
			}
			if err := enc.Encode(exportLine{Kind: "comment", Data: data}); err != nil {
				return err
			}
		}
		kvs, err := s.KVList(r.ctx)
		if err != nil {
			return err
		}
		for _, kv := range kvs {
			data, err := json.Marshal(kv)
			if err != nil {
				return err
			}
			if err := enc.Encode(exportLine{Kind: "kv", Data: data}); err != nil {
				return err
			}
		}
		crons, err := s.CronJobList(r.ctx)
		if err != nil {
			return err
		}
		for _, cj := range crons {
			data, err := json.Marshal(cj)
			if err != nil {
				return err
			}
			if err := enc.Encode(exportLine{Kind: "cron", Data: data}); err != nil {
				return err
			}
		}
		// Summary goes to stderr — stdout may be the JSONL stream
		// itself (`clu export > dump.jsonl`); mixing the two corrupts
		// the output. When writing to a file (-o) the summary is fine
		// either way, but stderr keeps the behaviour uniform.
		if !r.quiet {
			fmt.Fprintf(r.stderr, "exported %d issues, %d deps, %d comments, %d kv, %d cron\n",
				len(issues), len(deps), len(comments), len(kvs), len(crons))
		}
		return nil
	})
}
