package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rovak/beadsv2/internal/store"
	"github.com/rovak/beadsv2/internal/workflow"
)

// RunCmd instantiates a workflow template into issues + deps.
type RunCmd struct {
	Template string   `arg:"" help:"Template name (matches a file in .db/templates/)."`
	Var      []string `short:"v" name:"var" placeholder:"KEY=VALUE" help:"Variable bindings (repeatable)."`
	DryRun   bool     `name:"dry-run" help:"Validate and print the plan without writing to the DB."`
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
	plan, err := workflow.MakePlan(tmpl, vars)
	if err != nil {
		return err
	}
	if c.DryRun {
		return emitPlan(r, plan, "")
	}
	return withStore(r, func(s *store.Store) error {
		parent, err := s.Create(r.ctx, plan.Parent.Title, "task", 2, nil)
		if err != nil {
			return err
		}
		runLabel := "run:" + parent.ID
		if err := s.AddLabels(r.ctx, parent.ID, []string{runLabel, "template:" + plan.TemplateName}); err != nil {
			return err
		}
		stepIDs := map[string]string{} // step-id → issue-id
		for _, step := range plan.Steps {
			issue, err := s.Create(r.ctx, step.Title, step.Type, step.Priority, nil)
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

// checkpointPayload is the JSON shape stored in KV under "cp:<issue-id>".
type checkpointPayload struct {
	Kind      string   `json:"kind"`
	Approvers []string `json:"approvers,omitempty"`
}

func newCheckpointPayload(w *workflow.Wait) checkpointPayload {
	if w.Manual {
		return checkpointPayload{Kind: "manual"}
	}
	return checkpointPayload{Kind: "approval", Approvers: append([]string(nil), w.Approval...)}
}

// loadTemplate fetches a single named template from .db/templates/.
func loadTemplate(r *runCtx, name string) (workflow.Template, error) {
	dir := templatesDir(r)
	all, err := workflow.LoadDir(dir)
	if err != nil {
		return workflow.Template{}, err
	}
	t, ok := all[name]
	if !ok {
		return workflow.Template{}, fmt.Errorf("template %q not found in %s", name, dir)
	}
	return t, nil
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
			ID       string            `json:"id"`
			Title    string            `json:"title"`
			Type     string            `json:"type"`
			Priority int               `json:"priority"`
			Needs    []string          `json:"needs,omitempty"`
			Wait     *workflow.Wait    `json:"wait,omitempty"`
			IsLeaf   bool              `json:"is_leaf"`
		}
		type planOut struct {
			Template string            `json:"template"`
			Parent   string            `json:"parent,omitempty"`
			Title    string            `json:"title"`
			Vars     map[string]string `json:"vars,omitempty"`
			DryRun   bool              `json:"dry_run"`
			Steps    []stepOut         `json:"steps"`
		}
		out := planOut{
			Template: plan.TemplateName,
			Parent:   parentID,
			Title:    plan.Parent.Title,
			Vars:     plan.Vars,
			DryRun:   parentID == "",
		}
		for _, s := range plan.Steps {
			out.Steps = append(out.Steps, stepOut{
				ID: s.StepID, Title: s.Title, Type: s.Type,
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
