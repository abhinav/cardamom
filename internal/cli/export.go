package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/arjia-labs/clu/internal/store"
)

// exportLine is the wire format for JSONL. One per line. The "kind"
// field discriminates; "data" carries kind-specific fields.
//
// Issue:  {"kind":"issue","data":{...issue fields..., "labels":["x"]}}
// Dep:    {"kind":"dep","data":{"child":"bd-x","parent":"bd-y"}}
//
// Export carries portable *state* (issues, deps, labels, comments, kv) —
// not the audit log. Events are a local history trail and are
// intentionally excluded; restoring an export starts a fresh log.
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
	// export is a JSONL stream (one object per line) by design — it can't
	// honor the "--json = exactly one JSON value" contract. Reject the
	// flag explicitly rather than silently ignore it.
	if r.json {
		return errors.New("export always emits JSONL (one object per line); the --json flag does not apply — use plain `clu export`")
	}
	return withStore(r, func(s *store.Store) error {
		w := io.Writer(r.stdout)
		if c.Out != "" {
			f, err := os.Create(c.Out)
			if err != nil {
				return err
			}
			defer f.Close()
			w = f
		}
		counts, err := writeExportJSONL(r.ctx, s, w)
		if err != nil {
			return err
		}
		// Summary goes to stderr — stdout may be the JSONL stream
		// itself (`clu export > dump.jsonl`); mixing the two corrupts
		// the output. When writing to a file (-o) the summary is fine
		// either way, but stderr keeps the behaviour uniform.
		if !r.quiet {
			fmt.Fprintf(r.stderr, "exported %d issues, %d deps, %d comments, %d kv, %d cron\n",
				counts.Issues, counts.Deps, counts.Comments, counts.KV, counts.Cron)
		}
		return nil
	})
}

// exportCounts reports how many records of each kind a serialization emitted.
type exportCounts struct {
	Issues, Deps, Comments, KV, Cron int
}

// writeExportJSONL serializes the whole portable state of s as JSONL to w,
// one exportLine per record, in a deterministic order (issues, deps,
// comments, kv, cron). Shared by `clu export` and the git-ref sync path so
// both produce byte-identical snapshots from the same DB state. The audit
// log is intentionally excluded — see exportLine's doc comment.
func writeExportJSONL(ctx context.Context, s *store.Store, w io.Writer) (exportCounts, error) {
	var n exportCounts
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	emit := func(kind string, v any) error {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		return enc.Encode(exportLine{Kind: kind, Data: data})
	}

	issues, err := s.List(ctx, store.ListFilter{})
	if err != nil {
		return n, err
	}
	ids := make([]string, len(issues))
	for i, is := range issues {
		ids[i] = is.ID
	}
	labelsMap, err := s.LoadLabels(ctx, ids)
	if err != nil {
		return n, err
	}
	for _, is := range issues {
		if err := emit("issue", issueExport{Issue: is, Labels: labelsMap[is.ID]}); err != nil {
			return n, err
		}
	}
	n.Issues = len(issues)

	deps, err := s.AllDeps(ctx)
	if err != nil {
		return n, err
	}
	for _, d := range deps {
		if err := emit("dep", depExport{Child: d.ChildID, Parent: d.ParentID}); err != nil {
			return n, err
		}
	}
	n.Deps = len(deps)

	comments, err := s.AllComments(ctx)
	if err != nil {
		return n, err
	}
	for _, cm := range comments {
		if err := emit("comment", cm); err != nil {
			return n, err
		}
	}
	n.Comments = len(comments)

	kvs, err := s.KVList(ctx)
	if err != nil {
		return n, err
	}
	for _, kv := range kvs {
		if err := emit("kv", kv); err != nil {
			return n, err
		}
	}
	n.KV = len(kvs)

	crons, err := s.CronJobList(ctx)
	if err != nil {
		return n, err
	}
	for _, cj := range crons {
		if err := emit("cron", cj); err != nil {
			return n, err
		}
	}
	n.Cron = len(crons)

	return n, nil
}
