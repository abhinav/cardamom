package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/rovak/clu/internal/config"
	"github.com/rovak/clu/internal/store"
)

// BatchCmd instantiates a whole issue graph from a single JSON document in
// one transaction. The producer (any script / "codemode" / generator)
// emits the graph; clu validates it (acyclic, all refs resolve, fields
// valid), allocates real IDs, and writes it atomically. Built for scale —
// a thousand+ issues with complex deps in one shot.
//
//	node gen.js | clu batch --dry-run   # validate + stats, write nothing
//	node gen.js | clu batch --json      # commit; returns {alias: real-id}
type BatchCmd struct {
	File   string `arg:"" optional:"" help:"JSON file (default: stdin)."`
	DryRun bool   `name:"dry-run" help:"Validate and report stats without writing anything."`
	Agent  string `short:"a" name:"agent" default:"${user}" help:"Identity recorded as the actor on the created issues' audit events."`
}

// batchInput is the per-issue wire shape. Priority is a pointer so an
// omitted value defaults to 2 (normal) rather than 0 (highest).
type batchInput struct {
	Alias        string   `json:"alias"`
	Title        string   `json:"title"`
	Type         string   `json:"type,omitempty"`
	Priority     *int     `json:"priority,omitempty"`
	Assignee     *string  `json:"assignee,omitempty"`
	Description  *string  `json:"description,omitempty"`
	Notes        *string  `json:"notes,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	Needs        []string `json:"needs,omitempty"`
}

// batchDoc is the {"issues":[...]} wrapper form.
type batchDoc struct {
	Issues []batchInput `json:"issues"`
}

func (c *BatchCmd) Run(r *runCtx) error {
	raw, err := readBatchInput(c.File)
	if err != nil {
		return err
	}
	inputs, err := parseBatch(raw)
	if err != nil {
		return err
	}
	issues, err := toBatchIssues(inputs)
	if err != nil {
		return err
	}

	return withStore(r, func(s *store.Store) error {
		s.SetActor(c.Agent)
		if c.DryRun {
			stats, err := s.BatchValidate(r.ctx, issues)
			if err != nil {
				return err
			}
			if r.json {
				return r.emitJSON(map[string]any{"dry_run": true, "stats": stats})
			}
			r.notice("valid: ")
			printBatchStats(r, stats)
			return nil
		}
		mapping, stats, err := s.BatchCreate(r.ctx, issues)
		if err != nil {
			return err
		}
		if r.json {
			return r.emitJSON(map[string]any{
				"count":   stats.Issues,
				"edges":   stats.Edges,
				"created": mapping,
			})
		}
		printBatchStats(r, stats)
		r.notice("created %d issues\n", stats.Issues)
		return nil
	})
}

// readBatchInput reads the whole document from a file or stdin.
func readBatchInput(file string) ([]byte, error) {
	if file != "" {
		return os.ReadFile(file)
	}
	if isStdinTTY() {
		return nil, fmt.Errorf("no input: pass a JSON file or pipe a graph on stdin")
	}
	return io.ReadAll(os.Stdin)
}

// parseBatch accepts either a bare array of issues or {"issues":[...]}.
// Unknown fields are rejected so a typo'd key (e.g. "capabilites") fails
// loudly instead of silently dropping data.
func parseBatch(raw []byte) ([]batchInput, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty input")
	}
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(trimmed))
		d.DisallowUnknownFields()
		return d.Decode(v)
	}
	switch trimmed[0] {
	case '[':
		var arr []batchInput
		if err := dec(&arr); err != nil {
			return nil, fmt.Errorf("parse batch array: %w", err)
		}
		return arr, nil
	case '{':
		var doc batchDoc
		if err := dec(&doc); err != nil {
			return nil, fmt.Errorf("parse batch document: %w", err)
		}
		return doc.Issues, nil
	default:
		return nil, fmt.Errorf("input must be a JSON array or {\"issues\":[...]} object")
	}
}

// toBatchIssues maps wire inputs to store.BatchIssue, applying the
// priority default and validating capability names (charset enforced the
// same way `create --capability` does, so cap routing actually matches).
func toBatchIssues(inputs []batchInput) ([]store.BatchIssue, error) {
	out := make([]store.BatchIssue, len(inputs))
	for i, in := range inputs {
		priority := 2
		if in.Priority != nil {
			priority = *in.Priority
		}
		for _, cap := range in.Capabilities {
			if !config.ValidAgentOrCapName(cap) {
				ref := in.Alias
				if ref == "" {
					ref = fmt.Sprintf("#%d", i+1)
				}
				return nil, fmt.Errorf("%s: invalid capability %q (lowercase a-z, digits, dashes; start with a letter)", ref, cap)
			}
		}
		out[i] = store.BatchIssue{
			Alias:        in.Alias,
			Title:        in.Title,
			Type:         in.Type,
			Priority:     priority,
			Assignee:     in.Assignee,
			Description:  in.Description,
			Notes:        in.Notes,
			Capabilities: in.Capabilities,
			Labels:       in.Labels,
			Needs:        in.Needs,
		}
	}
	return out, nil
}

func printBatchStats(r *runCtx, st store.BatchStats) {
	r.notice("%d issues, %d edges (%d external), %d roots, %d leaves, depth %d\n",
		st.Issues, st.Edges, st.External, st.Roots, st.Leaves, st.MaxDepth)
}
