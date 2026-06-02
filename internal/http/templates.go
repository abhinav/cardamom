package http

import (
	"encoding/json"
	stdhttp "net/http"
	"sort"

	"github.com/Rovak/agents-clu/internal/store"
	"github.com/Rovak/agents-clu/internal/workflow"
)

// templatesUnavailable writes 503 when the server has no templates
// dir configured. Happens when the CLI didn't call WithTemplatesDir
// (shouldn't be reachable in production — `clu http` and `clu web`
// always set it — but the handler stays well-behaved if it does).
func (s *Server) templatesUnavailable(w stdhttp.ResponseWriter) {
	writeError(w, stdhttp.StatusServiceUnavailable, "templates directory not configured")
}

// loadTemplates wraps workflow.LoadDir for the HTTP layer. Returns
// LoadErrors alongside the healthy map so individual handlers can
// surface them in the response if useful.
func (s *Server) loadTemplates() (map[string]workflow.Template, []workflow.LoadError, error) {
	return workflow.LoadDir(s.templatesDir)
}

// templateVarOut mirrors workflow.Var but is serialised explicitly
// (omitempty on each field) so the form-builder client knows which
// hints to render.
type templateVarOut struct {
	Name     string `json:"name"`
	Label    string `json:"label,omitempty"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
	Pattern  string `json:"pattern,omitempty"`
}

// templateSummary is the list-shape — enough to render a card without
// the full step graph. Steps are loaded separately when the user opens
// the run dialog (and again on every plan dry-run).
type templateSummary struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Vars        []templateVarOut `json:"vars"`
	StepCount   int              `json:"step_count"`
}

// templateLoadErrOut surfaces a broken template (parse error, missing
// spec file, duplicate name) so the UI can flag it without losing the
// healthy ones.
type templateLoadErrOut struct {
	File  string `json:"file"`
	Error string `json:"error"`
}

type templatesListOut struct {
	Templates []templateSummary    `json:"templates"`
	Errors    []templateLoadErrOut `json:"errors,omitempty"`
}

// handleListTemplates — GET /api/templates
//
// Returns one summary per healthy template plus a side channel of
// LoadErrors so the UI can warn about broken YAML files.
// Templates are sorted alphabetically for deterministic rendering.
func (s *Server) handleListTemplates(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if s.templatesDir == "" {
		s.templatesUnavailable(w)
		return
	}
	tmpls, lerrs, err := s.loadTemplates()
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	names := make([]string, 0, len(tmpls))
	for n := range tmpls {
		names = append(names, n)
	}
	sort.Strings(names)
	out := templatesListOut{Templates: make([]templateSummary, 0, len(names))}
	for _, n := range names {
		out.Templates = append(out.Templates, summariseTemplate(tmpls[n]))
	}
	for _, e := range lerrs {
		out.Errors = append(out.Errors, templateLoadErrOut{File: e.File, Error: e.Err.Error()})
	}
	writeJSON(w, stdhttp.StatusOK, out)
}

// summariseTemplate trims a Template to the card-shape, sorting vars
// by (required-first, then alpha) so the form renders predictably.
func summariseTemplate(t workflow.Template) templateSummary {
	vs := make([]templateVarOut, 0, len(t.Vars))
	for name, v := range t.Vars {
		vs = append(vs, templateVarOut{
			Name:     name,
			Label:    v.Label,
			Default:  v.Default,
			Required: v.Required,
			Pattern:  v.Pattern,
		})
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].Required != vs[j].Required {
			return vs[i].Required
		}
		return vs[i].Name < vs[j].Name
	})
	return templateSummary{
		Name:        t.Name,
		Description: t.Description,
		Vars:        vs,
		StepCount:   len(t.Steps),
	}
}

// planRequest is the body of POST .../plan and .../run.
type planRequest struct {
	Vars map[string]string `json:"vars"`
}

// planStepOut mirrors workflow.StepSpec for the wire. Wait is left as
// the workflow type so its JSON shape is whatever yaml.Marshal +
// json.Marshal naturally produce; the UI treats it opaquely.
type planStepOut struct {
	StepID      string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Type        string         `json:"type"`
	Priority    int            `json:"priority"`
	Needs       []string       `json:"needs,omitempty"`
	Agent       string         `json:"agent,omitempty"`
	Wait        *workflow.Wait `json:"wait,omitempty"`
	IsLeaf      bool           `json:"is_leaf"`
}

type planOut struct {
	TemplateName string            `json:"template"`
	Title        string            `json:"title"`
	Vars         map[string]string `json:"vars"`
	Steps        []planStepOut     `json:"steps"`
}

// handlePlanTemplate — POST /api/templates/{name}/plan
//
// Dry-runs MakePlan against the caller-supplied vars and returns the
// resolved plan. Used by the run dialog to preview what hitting Run
// will create. Pure read — no writes, no IDs allocated.
//
// Var errors (missing required, pattern mismatch) come back as 400
// with the error string so the form can show inline feedback.
func (s *Server) handlePlanTemplate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if s.templatesDir == "" {
		s.templatesUnavailable(w)
		return
	}
	name := r.PathValue("name")
	t, ok, err := s.loadOne(name)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, stdhttp.StatusNotFound, "template not found: "+name)
		return
	}
	var body planRequest
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeError(w, stdhttp.StatusBadRequest, err.Error())
			return
		}
	}
	plan, err := workflow.MakePlan(t, body.Vars)
	if err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, stdhttp.StatusOK, planToOut(plan))
}

// handleRunTemplate — POST /api/templates/{name}/run
//
// Instantiates the workflow: creates the parent issue, each step
// issue, the dep edges between them, the run:/step:/checkpoint:
// labels, and the cp:<id> KV entries for any checkpoint steps. This
// mirrors RunCmd.Run in internal/cli/run.go — keep the two in sync
// when behaviour changes.
func (s *Server) handleRunTemplate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	if s.templatesDir == "" {
		s.templatesUnavailable(w)
		return
	}
	name := r.PathValue("name")
	t, ok, err := s.loadOne(name)
	if err != nil {
		writeError(w, stdhttp.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, stdhttp.StatusNotFound, "template not found: "+name)
		return
	}
	var body planRequest
	if r.ContentLength > 0 {
		if err := readJSON(r, &body); err != nil {
			writeError(w, stdhttp.StatusBadRequest, err.Error())
			return
		}
	}
	plan, err := workflow.MakePlan(t, body.Vars)
	if err != nil {
		writeError(w, stdhttp.StatusBadRequest, err.Error())
		return
	}
	parentID, err := s.instantiatePlan(r, plan)
	if err != nil {
		respondErr(w, err)
		return
	}
	out := planToOut(plan)
	writeJSON(w, stdhttp.StatusCreated, map[string]any{
		"parent_id": parentID,
		"plan":      out,
	})
}

// loadOne resolves a template by name. Doesn't fall back to file-path
// loading (unlike the CLI) — HTTP callers refer to templates by name.
func (s *Server) loadOne(name string) (workflow.Template, bool, error) {
	all, _, err := s.loadTemplates()
	if err != nil {
		return workflow.Template{}, false, err
	}
	t, ok := all[name]
	return t, ok, nil
}

// instantiatePlan walks the plan and creates parent + step issues
// + deps + labels + checkpoint KV. Mirrors RunCmd.Run minus the
// interactive prompts and emitPlan output. Returns the parent ID.
//
// Run-label naming, "template:" tag, checkpoint:pending convention,
// and parent-depends-on-leaves wiring all match the CLI exactly so
// the two paths produce structurally identical runs.
func (s *Server) instantiatePlan(r *stdhttp.Request, plan workflow.Plan) (string, error) {
	ctx := ctxOf(r)
	parent, err := s.store.CreateWithLinks(ctx, plan.Parent.Title, "task", 2, nil, store.CreateOpts{
		Description: plan.Parent.Description,
	})
	if err != nil {
		return "", err
	}
	runLabel := "run:" + parent.ID
	if _, err := s.store.AddLabels(ctx, parent.ID, []string{runLabel, "template:" + plan.TemplateName}); err != nil {
		return "", err
	}
	stepIDs := map[string]string{}
	for _, step := range plan.Steps {
		var agent *string
		if step.Agent != "" {
			a := step.Agent
			agent = &a
		}
		issue, err := s.store.CreateWithLinks(ctx, step.Title, step.Type, step.Priority, agent, store.CreateOpts{
			Description: step.Description,
		})
		if err != nil {
			return "", err
		}
		stepIDs[step.StepID] = issue.ID
		labels := []string{runLabel, "step:" + step.StepID}
		if step.Type == "checkpoint" {
			labels = append(labels, "checkpoint:pending")
		}
		if _, err := s.store.AddLabels(ctx, issue.ID, labels); err != nil {
			return "", err
		}
		for _, need := range step.Needs {
			if err := s.store.AddDep(ctx, issue.ID, stepIDs[need]); err != nil {
				return "", err
			}
		}
		if step.Type == "checkpoint" && step.Wait != nil {
			payload := store.CheckpointPayload{}
			if step.Wait.Manual {
				payload.Kind = "manual"
			} else {
				payload.Kind = "approval"
				payload.Approvers = append([]string(nil), step.Wait.Approval...)
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return "", err
			}
			if err := s.store.KVSet(ctx, "cp:"+issue.ID, string(raw)); err != nil {
				return "", err
			}
		}
	}
	// Parent depends on every leaf step so it only surfaces as ready
	// once the whole run completes.
	for _, step := range plan.Steps {
		if !step.IsLeaf {
			continue
		}
		if err := s.store.AddDep(ctx, parent.ID, stepIDs[step.StepID]); err != nil {
			return "", err
		}
	}
	return parent.ID, nil
}

// planToOut narrows workflow.Plan to the wire shape. workflow.Plan
// embeds the full StepSpec which is mostly the same; this exists for
// JSON tag stability and to filter out unused fields.
func planToOut(p workflow.Plan) planOut {
	steps := make([]planStepOut, len(p.Steps))
	for i, s := range p.Steps {
		steps[i] = planStepOut{
			StepID:      s.StepID,
			Title:       s.Title,
			Description: s.Description,
			Type:        s.Type,
			Priority:    s.Priority,
			Needs:       s.Needs,
			Agent:       s.Agent,
			Wait:        s.Wait,
			IsLeaf:      s.IsLeaf,
		}
	}
	return planOut{
		TemplateName: p.TemplateName,
		Title:        p.Parent.Title,
		Vars:         p.Vars,
		Steps:        steps,
	}
}
