package cli

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/rovak/beadsv2/internal/workflow"
)

// TemplateCmd is the parent for `clu template ls|show|validate`.
type TemplateCmd struct {
	Ls       TemplateLsCmd       `cmd:"" aliases:"list" help:"List available templates."`
	Show     TemplateShowCmd     `cmd:"" help:"Print one template as parsed YAML."`
	Validate TemplateValidateCmd `cmd:"" help:"Validate a template's structure."`
}

type TemplateLsCmd struct{}

func (c *TemplateLsCmd) Run(r *runCtx) error {
	all, err := workflow.LoadDir(templatesDir(r))
	if err != nil {
		return err
	}
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	if r.json {
		type row struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Steps       int    `json:"steps"`
		}
		out := make([]row, 0, len(names))
		for _, n := range names {
			t := all[n]
			out = append(out, row{Name: t.Name, Description: t.Description, Steps: len(t.Steps)})
		}
		return r.emitJSON(out)
	}
	if len(names) == 0 {
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
