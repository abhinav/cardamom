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
	File       string `arg:"" optional:"" help:"JSON file (default: stdin)."`
	DryRun     bool   `name:"dry-run" help:"Validate and report stats without writing anything."`
	Group      string `name:"group" help:"Wrap the batch under a parent umbrella issue with this title; every issue (and the parent) is tagged run:<parent-id>. Overrides a 'group' field in the document."`
	OnExisting string `name:"on-existing" enum:"skip,update" default:"skip" help:"When an issue's 'key' already exists: skip it (default) or update its title/type/priority/description from the source."`
	Agent      string `short:"a" name:"agent" default:"${user}" help:"Identity recorded as the actor on the created issues' audit events."`
}

// batchInput is the per-issue wire shape. Priority is a pointer so an
// omitted value defaults to 2 (normal) rather than 0 (highest).
type batchInput struct {
	Alias        string           `json:"alias"`
	Title        string           `json:"title"`
	Type         string           `json:"type,omitempty"`
	Priority     *int             `json:"priority,omitempty"`
	Assignee     *string          `json:"assignee,omitempty"`
	Description  *string          `json:"description,omitempty"`
	Notes        *string          `json:"notes,omitempty"`
	Capabilities []string         `json:"capabilities,omitempty"`
	Labels       []string         `json:"labels,omitempty"`
	Needs        []string         `json:"needs,omitempty"`
	Key          string           `json:"key,omitempty"`
	Checkpoint   *checkpointInput `json:"checkpoint,omitempty"`
}

// checkpointInput declares a manual gate. kind is optional — it's
// inferred as "approval" when approvers are listed, else "manual".
type checkpointInput struct {
	Kind      string   `json:"kind,omitempty"`
	Approvers []string `json:"approvers,omitempty"`
}

// batchDoc is the {"issues":[...]} wrapper form. An optional "group" wraps
// the batch under a parent umbrella; it may be a bare string (the title) or
// an object {"title","description"}.
type batchDoc struct {
	Group  json.RawMessage `json:"group,omitempty"`
	Issues []batchInput    `json:"issues"`
}

// batchGroupInput is the object form of a document "group".
type batchGroupInput struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

func (c *BatchCmd) Run(r *runCtx) error {
	raw, err := readBatchInput(c.File)
	if err != nil {
		return err
	}
	inputs, docGroup, err := parseBatch(raw)
	if err != nil {
		return err
	}
	issues, err := toBatchIssues(inputs)
	if err != nil {
		return err
	}
	// Group resolution: --group flag wins over a document "group".
	var group *store.BatchGroup
	if c.Group != "" {
		group = &store.BatchGroup{Title: c.Group}
	} else if docGroup != nil {
		group = &store.BatchGroup{Title: docGroup.Title, Description: docGroup.Description}
	}
	mode := store.BatchSkip
	if c.OnExisting == "update" {
		mode = store.BatchUpdate
	}

	return withStore(r, func(s *store.Store) error {
		s.SetActor(c.Agent)
		if c.DryRun {
			stats, err := s.BatchValidate(r.ctx, issues)
			if err != nil {
				return err
			}
			if r.json {
				return r.emitJSON(map[string]any{"dry_run": true, "stats": stats, "grouped": group != nil})
			}
			r.notice("valid: ")
			printBatchStats(r, stats)
			if stats.Existing > 0 {
				r.notice("%d already exist (would %s)\n", stats.Existing, c.OnExisting)
			}
			if group != nil {
				r.notice("group: %s (parent issue would be created)\n", group.Title)
			}
			return nil
		}
		res, err := s.BatchCreate(r.ctx, issues, group, mode)
		if err != nil {
			return err
		}
		if r.json {
			out := map[string]any{
				"count":    res.Stats.Issues,
				"edges":    res.Stats.Edges,
				"new":      res.New,
				"existing": res.Existing,
				"updated":  res.Updated,
				"created":  res.Mapping,
			}
			if res.ParentID != "" {
				out["group"] = res.ParentID
			}
			return r.emitJSON(out)
		}
		printBatchStats(r, res.Stats)
		summary := fmt.Sprintf("created %d new", res.New)
		if mode == store.BatchUpdate {
			summary += fmt.Sprintf(", updated %d existing", res.Updated)
		} else if res.Existing > 0 {
			summary += fmt.Sprintf(", skipped %d existing", res.Existing)
		}
		if res.ParentID != "" {
			summary += fmt.Sprintf(" under group %s", res.ParentID)
		}
		r.notice("%s\n", summary)
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
// loudly instead of silently dropping data. Returns the optional document
// group (nil for the array form or when absent).
func parseBatch(raw []byte) ([]batchInput, *batchGroupInput, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil, fmt.Errorf("empty input")
	}
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(trimmed))
		d.DisallowUnknownFields()
		if err := d.Decode(v); err != nil {
			return err
		}
		// Reject trailing tokens after the value — a generator that leaks an
		// extra log line or a second JSON value onto stdout must fail loudly,
		// not silently commit a partial-looking stream.
		if d.More() {
			return fmt.Errorf("unexpected trailing content after the JSON value")
		}
		return nil
	}
	switch trimmed[0] {
	case '[':
		var arr []batchInput
		if err := dec(&arr); err != nil {
			return nil, nil, fmt.Errorf("parse batch array: %w", err)
		}
		return arr, nil, nil
	case '{':
		var doc batchDoc
		if err := dec(&doc); err != nil {
			return nil, nil, fmt.Errorf("parse batch document: %w", err)
		}
		group, err := parseGroup(doc.Group)
		if err != nil {
			return nil, nil, err
		}
		return doc.Issues, group, nil
	default:
		return nil, nil, fmt.Errorf("input must be a JSON array or {\"issues\":[...]} object")
	}
}

// parseGroup resolves a document "group" value, which may be a bare string
// (the title) or an object {"title","description"}.
func parseGroup(raw json.RawMessage) (*batchGroupInput, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return nil, nil
	}
	switch t[0] {
	case '"':
		var title string
		if err := json.Unmarshal(t, &title); err != nil {
			return nil, fmt.Errorf("parse group: %w", err)
		}
		return &batchGroupInput{Title: title}, nil
	case '{':
		var g batchGroupInput
		d := json.NewDecoder(bytes.NewReader(t))
		d.DisallowUnknownFields()
		if err := d.Decode(&g); err != nil {
			return nil, fmt.Errorf("parse group: %w", err)
		}
		return &g, nil
	default:
		return nil, fmt.Errorf("group must be a string title or {\"title\",\"description\"} object")
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
		var cp *store.CheckpointPayload
		if in.Checkpoint != nil {
			kind := in.Checkpoint.Kind
			if kind == "" {
				if len(in.Checkpoint.Approvers) > 0 {
					kind = "approval"
				} else {
					kind = "manual"
				}
			}
			cp = &store.CheckpointPayload{Kind: kind, Approvers: in.Checkpoint.Approvers}
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
			Key:          in.Key,
			Checkpoint:   cp,
		}
	}
	return out, nil
}

func printBatchStats(r *runCtx, st store.BatchStats) {
	r.notice("%d issues, %d edges (%d external), %d roots, %d leaves, depth %d, %d checkpoints\n",
		st.Issues, st.Edges, st.External, st.Roots, st.Leaves, st.MaxDepth, st.Checkpoints)
}
