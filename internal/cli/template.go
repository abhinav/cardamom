package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rovak/clu/internal/workflow"
)

// TemplateCmd is the parent for `clu template ls|show|validate`.
type TemplateCmd struct {
	Ls       TemplateLsCmd       `cmd:"" aliases:"list" help:"List available templates."`
	Show     TemplateShowCmd     `cmd:"" help:"Print one template as parsed YAML."`
	Validate TemplateValidateCmd `cmd:"" help:"Validate a template's structure."`
}

type TemplateLsCmd struct{}

func (c *TemplateLsCmd) Run(r *runCtx) error {
	all, loadErrs, err := workflow.LoadDir(templatesDir(r))
	if err != nil {
		return err
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	if r.json {
		type errOut struct {
			File  string `json:"file"`
			Error string `json:"error"`
		}
		type row struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Steps       int    `json:"steps"`
		}
		type out struct {
			Templates []row    `json:"templates"`
			Errors    []errOut `json:"errors,omitempty"`
		}
		rows := make([]row, 0, len(names))
		for _, n := range names {
			t := all[n]
			rows = append(rows, row{Name: t.Name, Description: t.Description, Steps: len(t.Steps)})
		}
		errs := make([]errOut, 0, len(loadErrs))
		for _, e := range loadErrs {
			errs = append(errs, errOut{File: e.File, Error: e.Err.Error()})
		}
		return r.emitJSON(out{Templates: rows, Errors: errs})
	}
	if len(names) == 0 && len(loadErrs) == 0 {
		fmt.Fprintln(r.stdout, "(none)")
		return nil
	}
	for _, n := range names {
		t := all[n]
		if t.Description != "" {
			fmt.Fprintf(r.stdout, "%-20s  %d step(s)  %s\n", t.Name, len(t.Steps), t.Description)
		} else {
			fmt.Fprintf(r.stdout, "%-20s  %d step(s)\n", t.Name, len(t.Steps))
		}
	}
	// Surface broken templates after the healthy listing so the
	// operator can see which file to fix without having to grep the
	// dir manually. stderr-bound; doesn't pollute stdout's data.
	for _, e := range loadErrs {
		fmt.Fprintf(r.stderr, "warning: %s: %s\n", e.File, e.Err)
	}
	return nil
}

type TemplateShowCmd struct {
	Name string `arg:"" help:"Template name (in .clu/templates/) or path to a .yaml file."`
}

func (c *TemplateShowCmd) Run(r *runCtx) error {
	t, err := loadTemplate(r, c.Name)
	if err != nil {
		return err
	}
	if r.json {
		return r.emitJSON(t)
	}
	enc := json.NewEncoder(r.stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

type TemplateValidateCmd struct {
	Name string `arg:"" help:"Template name (in .clu/templates/) or path to a .yaml file."`
}

func (c *TemplateValidateCmd) Run(r *runCtx) error {
	t, err := loadTemplate(r, c.Name)
	if err != nil {
		return err
	}
	if err := t.Validate(); err != nil {
		return err
	}
	r.notice("ok: %s\n", t.Name)
	return nil
}
