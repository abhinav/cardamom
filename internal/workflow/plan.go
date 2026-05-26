package workflow

import (
	"fmt"
	"sort"
)

// Plan is the abstract result of applying a Template to a set of vars.
// It is pure — no IDs allocated, no DB writes performed. The CLI layer
// walks the plan, calls store.Create per step, and writes deps/labels/KV.
type Plan struct {
	TemplateName string
	Vars         map[string]string
	Parent       ParentSpec
	Steps        []StepSpec
}

// ParentSpec describes the root issue of a run.
type ParentSpec struct {
	Title string
}

// StepSpec describes one step in a Plan.
type StepSpec struct {
	StepID   string   // template step id (e.g. "build")
	Title    string   // interpolated
	Type     string   // "task" or "checkpoint"
	Priority int
	Needs    []string // step ids, not issue ids
	Agent    string   // pre-assigned agent lane; "" = unassigned
	Wait     *Wait    // checkpoint config; nil for tasks
	IsLeaf   bool     // no other step needs this one — parent depends on it
}

// MakePlan validates the template, resolves vars, interpolates titles,
// and produces a Plan. It does not touch any external state.
func MakePlan(t Template, in map[string]string) (Plan, error) {
	if err := t.Validate(); err != nil {
		return Plan{}, err
	}
	vars, err := t.ResolveVars(in)
	if err != nil {
		return Plan{}, err
	}
	steps := make([]StepSpec, 0, len(t.Steps))
	needed := map[string]bool{}
	for _, s := range t.Steps {
		for _, n := range s.Needs {
			needed[n] = true
		}
	}
	for _, s := range t.Steps {
		title, err := Interpolate(s.Title, vars)
		if err != nil {
			return Plan{}, fmt.Errorf("step %s: %w", s.ID, err)
		}
		pr := 2
		if s.Priority != nil {
			pr = *s.Priority
		}
		typ := s.Type
		if typ == "" {
			typ = "task"
		}
		wait, err := interpolateWait(s.Wait, vars)
		if err != nil {
			return Plan{}, fmt.Errorf("step %s: wait: %w", s.ID, err)
		}
		steps = append(steps, StepSpec{
			StepID:   s.ID,
			Title:    title,
			Type:     typ,
			Priority: pr,
			Needs:    append([]string(nil), s.Needs...),
			Agent:    s.Agent,
			Wait:     wait,
			IsLeaf:   !needed[s.ID],
		})
	}
	return Plan{
		TemplateName: t.Name,
		Vars:         vars,
		Parent:       ParentSpec{Title: parentTitle(t, vars)},
		Steps:        steps,
	}, nil
}

// interpolateWait runs {{var}} substitution on every string inside a
// Wait clause (currently just the approvers list). Returns nil if w is
// nil; otherwise returns a fresh Wait with substituted values.
func interpolateWait(w *Wait, vars map[string]string) (*Wait, error) {
	if w == nil {
		return nil, nil
	}
	out := &Wait{Manual: w.Manual}
	if len(w.Approval) > 0 {
		out.Approval = make([]string, len(w.Approval))
		for i, a := range w.Approval {
			v, err := Interpolate(a, vars)
			if err != nil {
				return nil, err
			}
			out.Approval[i] = v
		}
	}
	return out, nil
}

// parentTitle produces e.g. "release 1.2.3" by appending var values
// (sorted by var name) to the template name.
func parentTitle(t Template, vars map[string]string) string {
	if len(vars) == 0 {
		return t.Name
	}
	names := make([]string, 0, len(t.Vars))
	for n := range t.Vars {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := []string{t.Name}
	for _, n := range names {
		if v, ok := vars[n]; ok {
			parts = append(parts, v)
		}
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " " + p
	}
	return out
}
