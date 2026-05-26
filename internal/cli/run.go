package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rovak/clu/internal/store"
	"github.com/rovak/clu/internal/workflow"
)

// RunCmd instantiates a workflow template into issues + deps.
type RunCmd struct {
	Template string   `arg:"" help:"Template name (in .clu/templates/) or path to a .yaml file."`
	Var      []string `short:"v" name:"var" placeholder:"KEY=VALUE" help:"Variable bindings (repeatable)."`
	DryRun   bool     `name:"dry-run" help:"Validate and print the plan without writing to the DB."`
	NoPrompt bool     `name:"no-prompt" help:"Don't prompt for missing required vars; fail instead. Default: prompt when stdin is a TTY."`
}

func (c *RunCmd) Run(r *runCtx) error {
	vars, err := parseVarPairs(c.Var)
	if err != nil {
		return err
	}
	tmpl, err := loadTemplate(r, c.Template)
	if err != nil {
		return err
	}
	// Fill missing required vars interactively when stdin is a TTY and
	// the user didn't pass --no-prompt. Skipped under --json (one JSON
	// document in, one out — no room for human dialog) and --quiet.
	if !c.NoPrompt && !r.json && !r.quiet && isStdinTTY() {
		if err := promptMissingVars(r, tmpl, vars); err != nil {
			return err
		}
	}
	plan, err := workflow.MakePlan(tmpl, vars)
	if err != nil {
		return err
	}
	if c.DryRun {
		return emitPlan(r, plan, "")
	}
	return withStore(r, func(s *store.Store) error {
		parent, err := s.CreateWithLinks(r.ctx, plan.Parent.Title, "task", 2, nil, store.CreateOpts{
			Description: plan.Parent.Description,
		})
		if err != nil {
			return err
		}
		runLabel := "run:" + parent.ID
		if err := s.AddLabels(r.ctx, parent.ID, []string{runLabel, "template:" + plan.TemplateName}); err != nil {
			return err
		}
		stepIDs := map[string]string{} // step-id → issue-id
		for _, step := range plan.Steps {
			issue, err := s.CreateWithLinks(r.ctx, step.Title, step.Type, step.Priority, agentPtr(step.Agent), store.CreateOpts{
				Description: step.Description,
			})
			if err != nil {
				return err
			}
			stepIDs[step.StepID] = issue.ID
			labels := []string{runLabel, "step:" + step.StepID}
			if step.Type == "checkpoint" {
				labels = append(labels, "checkpoint:pending")
			}
			if err := s.AddLabels(r.ctx, issue.ID, labels); err != nil {
				return err
			}
			for _, need := range step.Needs {
				if err := s.AddDep(r.ctx, issue.ID, stepIDs[need]); err != nil {
					return err
				}
			}
			if step.Type == "checkpoint" && step.Wait != nil {
				payload, _ := json.Marshal(newCheckpointPayload(step.Wait))
				if err := s.KVSet(r.ctx, "cp:"+issue.ID, string(payload)); err != nil {
					return err
				}
			}
		}
		// Parent depends on every leaf so it surfaces in `ready`
		// only once all steps are closed.
		for _, step := range plan.Steps {
			if !step.IsLeaf {
				continue
			}
			if err := s.AddDep(r.ctx, parent.ID, stepIDs[step.StepID]); err != nil {
				return err
			}
		}
		return emitPlan(r, plan, parent.ID)
	})
}

// isStdinTTY reports whether stdin is connected to a terminal (not a
// pipe / file / heredoc). Used to gate interactive var prompting so
// scripts and CI invocations fail fast instead of hanging on input.
func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// promptMissingVars asks for any required var not already in `vars` —
// mutates `vars` in place. Defaults are accepted by hitting enter.
// Pattern validation is enforced; bad input re-prompts rather than
// aborting the whole run.
func promptMissingVars(r *runCtx, t workflow.Template, vars map[string]string) error {
	names := make([]string, 0, len(t.Vars))
	for n := range t.Vars {
		names = append(names, n)
	}
	sortVarNames(names, t.Vars)

	reader := bufio.NewReader(os.Stdin)
	first := true
	for _, name := range names {
		if _, set := vars[name]; set {
			continue
		}
		v := t.Vars[name]
		// Skip non-required vars with no value supplied — ResolveVars
		// applies defaults or leaves them unset.
		if !v.Required && v.Default == "" {
			continue
		}
		if !v.Required {
			continue
		}
		if first {
			fmt.Fprintf(r.stderr, "→ %s needs some inputs:\n", t.Name)
			first = false
		}
		val, err := readVar(reader, r.stderr, name, v)
		if err != nil {
			return err
		}
		vars[name] = val
	}
	return nil
}

// readVar prompts once for `name`, re-prompting on pattern mismatch.
// EOF mid-prompt → error (caller's stdin was closed; we can't continue).
func readVar(reader *bufio.Reader, w io.Writer, name string, v workflow.Var) (string, error) {
	label := v.Prompt
	if label == "" {
		label = name
	}
	var prompt string
	if v.Default != "" {
		prompt = fmt.Sprintf("  %s [%s]: ", label, v.Default)
	} else {
		prompt = fmt.Sprintf("  %s: ", label)
	}
	for {
		fmt.Fprint(w, prompt)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && line == "" {
				return "", fmt.Errorf("stdin closed before %s was supplied", name)
			}
			if err != io.EOF {
				return "", err
			}
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" && v.Default != "" {
			line = v.Default
		}
		if line == "" {
			fmt.Fprintln(w, "    (required — please enter a value)")
			continue
		}
		if v.Pattern != "" {
			re, perr := regexp.Compile(v.Pattern)
			if perr != nil {
				return "", fmt.Errorf("var %s: invalid pattern: %w", name, perr)
			}
			if !re.MatchString(line) {
				fmt.Fprintf(w, "    (must match %s)\n", v.Pattern)
				continue
			}
		}
		return line, nil
	}
}

// sortVarNames orders required vars first (alpha within), then optional.
// Deterministic + puts mandatory inputs up front.
func sortVarNames(names []string, defs map[string]workflow.Var) {
	type rec struct {
		name     string
		required bool
	}
	recs := make([]rec, len(names))
	for i, n := range names {
		recs[i] = rec{n, defs[n].Required}
	}
	for i := 1; i < len(recs); i++ {
		for j := i; j > 0; j-- {
			a, b := recs[j-1], recs[j]
			if a.required == b.required {
				if a.name <= b.name {
					break
				}
			} else if a.required {
				break
			}
			recs[j-1], recs[j] = b, a
		}
	}
	for i, rcd := range recs {
		names[i] = rcd.name
	}
}

// newCheckpointPayload converts a workflow.Wait into the KV shape the
// store package understands. Kept in cli/ because it depends on the
// workflow package (which store deliberately doesn't import).
func newCheckpointPayload(w *workflow.Wait) store.CheckpointPayload {
	if w.Manual {
		return store.CheckpointPayload{Kind: "manual"}
	}
	return store.CheckpointPayload{Kind: "approval", Approvers: append([]string(nil), w.Approval...)}
}

// loadTemplate resolves a template by name or by file path.
//
// If `ref` contains a path separator or ends in .yaml/.yml, it is loaded
// directly as a file (relative paths are resolved against the process
// cwd, not the DB dir). Otherwise it's treated as a name and looked up
// in .clu/templates/.
func loadTemplate(r *runCtx, ref string) (workflow.Template, error) {
	if looksLikePath(ref) {
		return workflow.Load(ref)
	}
	dir := templatesDir(r)
	all, err := workflow.LoadDir(dir)
	if err != nil {
		return workflow.Template{}, err
	}
	t, ok := all[ref]
	if !ok {
		return workflow.Template{}, fmt.Errorf("template %q not found in %s (or pass a path ending in .yaml)", ref, dir)
	}
	return t, nil
}

// looksLikePath returns true if ref is clearly a file path rather than
// a template name.
func looksLikePath(ref string) bool {
	if strings.ContainsAny(ref, "/\\") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(ref))
	return ext == ".yaml" || ext == ".yml"
}

func templatesDir(r *runCtx) string {
	return filepath.Join(r.dir, "templates")
}

// parseVarPairs accepts ["k=v", "x=y"] and returns a map. Errors on
// missing '=' or empty key.
func parseVarPairs(pairs []string) (map[string]string, error) {
	out := map[string]string{}
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --var %q (expected KEY=VALUE)", p)
		}
		out[p[:eq]] = p[eq+1:]
	}
	return out, nil
}

// emitPlan prints a human or JSON summary of a Plan. parentID is "" in
// --dry-run mode.
func emitPlan(r *runCtx, plan workflow.Plan, parentID string) error {
	if r.json {
		type stepOut struct {
			ID          string         `json:"id"`
			Title       string         `json:"title"`
			Description string         `json:"description,omitempty"`
			Type        string         `json:"type"`
			Priority    int            `json:"priority"`
			Needs       []string       `json:"needs,omitempty"`
			Wait        *workflow.Wait `json:"wait,omitempty"`
			IsLeaf      bool           `json:"is_leaf"`
		}
		type planOut struct {
			Template string            `json:"template"`
			Parent   string            `json:"parent,omitempty"`
			Title    string            `json:"title"`
			Spec     string            `json:"spec,omitempty"`
			Vars     map[string]string `json:"vars,omitempty"`
			DryRun   bool              `json:"dry_run"`
			Steps    []stepOut         `json:"steps"`
		}
		out := planOut{
			Template: plan.TemplateName,
			Parent:   parentID,
			Title:    plan.Parent.Title,
			Spec:     plan.Spec,
			Vars:     plan.Vars,
			DryRun:   parentID == "",
		}
		for _, s := range plan.Steps {
			out.Steps = append(out.Steps, stepOut{
				ID: s.StepID, Title: s.Title, Description: s.Description, Type: s.Type,
				Priority: s.Priority, Needs: s.Needs, Wait: s.Wait, IsLeaf: s.IsLeaf,
			})
		}
		return r.emitJSON(out)
	}
	if parentID == "" {
		fmt.Fprintf(r.stdout, "dry-run: %s\n", plan.Parent.Title)
	} else {
		fmt.Fprintf(r.stdout, "%s  %s\n", parentID, plan.Parent.Title)
	}
	for _, s := range plan.Steps {
		fmt.Fprintf(r.stdout, "  %-16s  %-10s  %s", s.StepID, s.Type, s.Title)
		if len(s.Needs) > 0 {
			fmt.Fprintf(r.stdout, "  needs: %s", strings.Join(s.Needs, ","))
		}
		fmt.Fprintln(r.stdout)
	}
	return nil
}
