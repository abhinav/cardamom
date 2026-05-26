// Package workflow loads YAML workflow templates and turns them into
// concrete issue/dep/label plans.
//
// A template is a declarative description of a multi-step process.
// `Load` parses YAML; `MakePlan` produces an instantiation plan from a
// template and a set of variable bindings; the CLI layer writes the
// plan to the store.
package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Template is a parsed workflow definition.
type Template struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	// Spec is shared context prepended to every step's description on
	// instantiation. Inline string, or @<path> to load from a file
	// relative to the template (so a 50-line scaffold-spec.md can live
	// next to the template). {{var}} placeholders interpolate.
	Spec  string         `yaml:"spec,omitempty"`
	Vars  map[string]Var `yaml:"vars,omitempty"`
	Steps []Step         `yaml:"steps"`
}

// Var is a single variable declaration.
type Var struct {
	Required bool   `yaml:"required,omitempty"`
	Default  string `yaml:"default,omitempty"`
	Pattern  string `yaml:"pattern,omitempty"`
	// Label is a short human-readable name for the variable. Used by
	// the interactive prompt today; future surfaces (docs, GUI, JSON
	// schemas) will reuse the same string. Defaults to the var name
	// when empty.
	Label string `yaml:"label,omitempty"`
}

// Step is one step in a workflow.
type Step struct {
	ID    string `yaml:"id"`
	Title string `yaml:"title"`
	// Description is per-step instructions — acceptance criteria, the
	// specific job, any references downstream agents will need.
	// {{var}} interpolation applies. The template's spec is prepended
	// at instantiation so each step issue is self-contained.
	Description string   `yaml:"description,omitempty"`
	Type        string   `yaml:"type,omitempty"`
	Priority    *int     `yaml:"priority,omitempty"`
	Needs       []string `yaml:"needs,omitempty"`
	Agent       string   `yaml:"agent,omitempty"` // pre-assigns the step to an agent lane
	Wait        *Wait    `yaml:"wait,omitempty"`
}

// Wait describes the blocking condition for a checkpoint step.
type Wait struct {
	Manual   bool     `yaml:"manual,omitempty"`
	Approval []string `yaml:"approval,omitempty"`
}

var (
	idRe      = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	varNameRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	interpRe  = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)
)

// Load reads and parses a template file. If `spec:` starts with @,
// the rest is treated as a path (resolved relative to the template
// file) and its contents replace the @-string. Useful for keeping
// long shared-context blocks in a sibling markdown file.
func Load(path string) (Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Template{}, err
	}
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Template{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if t.Name == "" {
		t.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if strings.HasPrefix(t.Spec, "@") {
		ref := strings.TrimSpace(t.Spec[1:])
		specPath := ref
		if !filepath.IsAbs(specPath) {
			specPath = filepath.Join(filepath.Dir(path), ref)
		}
		body, err := os.ReadFile(specPath)
		if err != nil {
			return Template{}, fmt.Errorf("%s: spec %s: %w", filepath.Base(path), ref, err)
		}
		t.Spec = string(body)
	}
	return t, nil
}

// LoadDir loads all *.yaml/*.yml files from dir, keyed by template name.
// Missing dir returns an empty map, not an error.
func LoadDir(dir string) (map[string]Template, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Template{}, nil
		}
		return nil, err
	}
	out := map[string]Template{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		t, err := Load(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		if _, dup := out[t.Name]; dup {
			return nil, fmt.Errorf("duplicate template name %q (in %s)", t.Name, e.Name())
		}
		out[t.Name] = t
	}
	return out, nil
}

// Validate runs structural checks.
func (t Template) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("template name required")
	}
	for name, v := range t.Vars {
		if !varNameRe.MatchString(name) {
			return fmt.Errorf("invalid var name %q", name)
		}
		if v.Pattern != "" {
			if _, err := regexp.Compile(v.Pattern); err != nil {
				return fmt.Errorf("var %s: invalid pattern: %w", name, err)
			}
		}
	}
	if len(t.Steps) == 0 {
		return fmt.Errorf("template has no steps")
	}
	seen := map[string]bool{}
	for _, s := range t.Steps {
		if !idRe.MatchString(s.ID) {
			return fmt.Errorf("invalid step id %q (must be kebab-case, lowercase)", s.ID)
		}
		if seen[s.ID] {
			return fmt.Errorf("duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
		if s.Title == "" {
			return fmt.Errorf("step %s: title required", s.ID)
		}
		switch s.Type {
		case "", "task":
			if s.Wait != nil {
				return fmt.Errorf("step %s: 'wait' only valid for checkpoint steps", s.ID)
			}
		case "checkpoint":
			if s.Wait == nil {
				return fmt.Errorf("step %s: checkpoint requires a 'wait' clause", s.ID)
			}
			if !s.Wait.Manual && len(s.Wait.Approval) == 0 {
				return fmt.Errorf("step %s: wait must be 'manual: true' or have approvers", s.ID)
			}
		default:
			return fmt.Errorf("step %s: unknown type %q (want task or checkpoint)", s.ID, s.Type)
		}
	}
	for _, s := range t.Steps {
		for _, n := range s.Needs {
			if !seen[n] {
				return fmt.Errorf("step %s: needs unknown step %q", s.ID, n)
			}
			if n == s.ID {
				return fmt.Errorf("step %s: cannot depend on itself", s.ID)
			}
		}
	}
	return detectCycle(t.Steps)
}

func detectCycle(steps []Step) error {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	deps := map[string][]string{}
	for _, s := range steps {
		deps[s.ID] = s.Needs
		color[s.ID] = white
	}
	var visit func(string) error
	visit = func(id string) error {
		switch color[id] {
		case grey:
			return fmt.Errorf("cycle detected involving step %q", id)
		case black:
			return nil
		}
		color[id] = grey
		for _, n := range deps[id] {
			if err := visit(n); err != nil {
				return err
			}
		}
		color[id] = black
		return nil
	}
	ids := make([]string, 0, len(steps))
	for _, s := range steps {
		ids = append(ids, s.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// ResolveVars validates caller-supplied values against the template's
// declared vars. Returns the merged map (defaults applied).
func (t Template) ResolveVars(in map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for name, v := range t.Vars {
		val, present := in[name]
		if !present {
			if v.Default != "" {
				val = v.Default
			} else if v.Required {
				return nil, fmt.Errorf("var %s is required", name)
			} else {
				continue
			}
		}
		if v.Pattern != "" {
			re, err := regexp.Compile(v.Pattern)
			if err != nil {
				return nil, fmt.Errorf("var %s: invalid pattern: %w", name, err)
			}
			if !re.MatchString(val) {
				return nil, fmt.Errorf("var %s: value %q does not match %s", name, val, v.Pattern)
			}
		}
		out[name] = val
	}
	for name := range in {
		if _, declared := t.Vars[name]; !declared {
			return nil, fmt.Errorf("unknown var %q (not declared in template)", name)
		}
	}
	return out, nil
}

// Interpolate substitutes {{var}} placeholders in s.
// An unknown var name is an error.
func Interpolate(s string, vars map[string]string) (string, error) {
	var firstErr error
	out := interpRe.ReplaceAllStringFunc(s, func(match string) string {
		name := interpRe.FindStringSubmatch(match)[1]
		v, ok := vars[name]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unknown var %q", name)
			}
			return match
		}
		return v
	})
	return out, firstErr
}
