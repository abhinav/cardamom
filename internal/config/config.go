// Package config loads and validates the per-project `config.yaml` that
// lives at <dir>/config.yaml (default <dir> = ".clu").
//
// The file is human-edited and committed to git alongside workflow
// templates. Local-only state (the SQLite database, WAL files) lives in
// the same directory but is gitignored — see the snippet that
// `clu init` writes alongside the config.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Config is the parsed shape of <dir>/config.yaml. New top-level fields
// MUST round-trip through yaml.v3 with strict unmarshalling (unknown
// keys are an error — see Load) so typos surface immediately.
type Config struct {
	IDPrefix string           `yaml:"id_prefix"`
	Agents   map[string]Agent `yaml:"agents,omitempty"`
}

// Agent is the declarative side of an agent: who they are and what they
// can do. Committed to git. The live side (heartbeat, pid, host) lives
// in the active_agents table, populated by --wait/--watch loops.
type Agent struct {
	Description  string   `yaml:"description,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
}

// Default returns a Config with the safe defaults Load uses when a key
// is missing. Callers wanting Init-time defaults should start here.
func Default() Config {
	return Config{
		IDPrefix: "clu-",
	}
}

// idPrefixRe enforces the allowed shape: lowercase + digits + dashes,
// must END in '-' so the boundary between prefix and hex suffix is
// unambiguous when reading IDs back. Maximum 16 chars total so IDs stay
// readable.
var idPrefixRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-$`)

// agentNameRe restricts agent + capability names to safe shell tokens.
// No spaces, no underscores, lowercase + digits + dashes only.
var agentNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate enforces the rules described on each field. Returned errors
// are user-facing and meant to land directly in CLI stderr.
func (c Config) Validate() error {
	if c.IDPrefix == "" {
		return errors.New("id_prefix: required")
	}
	if len(c.IDPrefix) > 16 {
		return fmt.Errorf("id_prefix %q: too long (max 16 chars)", c.IDPrefix)
	}
	if !idPrefixRe.MatchString(c.IDPrefix) {
		return fmt.Errorf("id_prefix %q: must be lowercase a-z, 0-9, dashes; must end with `-`", c.IDPrefix)
	}
	for name, a := range c.Agents {
		if !agentNameRe.MatchString(name) {
			return fmt.Errorf("agent %q: name must be lowercase a-z, digits, dashes; start with a letter", name)
		}
		for _, cap := range a.Capabilities {
			if !agentNameRe.MatchString(cap) {
				return fmt.Errorf("agent %q: capability %q: same rules as agent names", name, cap)
			}
		}
	}
	return nil
}

// AgentsWithCapability returns every declared agent name that lists the
// given capability. Useful for "who could pick this up?" queries.
func (c Config) AgentsWithCapability(cap string) []string {
	var out []string
	for name, a := range c.Agents {
		for _, ac := range a.Capabilities {
			if ac == cap {
				out = append(out, name)
				break
			}
		}
	}
	return out
}

// Path returns the conventional config path inside a project directory.
func Path(dir string) string {
	return filepath.Join(dir, "config.yaml")
}

// Load reads and parses <dir>/config.yaml. Missing file → returns Default
// (no error); present file → strict-parse (unknown keys = error) and
// validate. Errors include the file path for easy navigation.
func Load(dir string) (Config, error) {
	out := Default()
	path := Path(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("%s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true) // typos are loud, not silent
	var parsed Config
	if err := dec.Decode(&parsed); err != nil {
		return out, fmt.Errorf("%s: %w", path, err)
	}
	if parsed.IDPrefix == "" {
		parsed.IDPrefix = out.IDPrefix
	}
	if err := parsed.Validate(); err != nil {
		return out, fmt.Errorf("%s: %w", path, err)
	}
	return parsed, nil
}

// Write serialises c to <dir>/config.yaml with leading comments
// explaining each knob. Intended for `clu init` on a fresh project.
// Refuses to overwrite an existing file — callers handle the "already
// exists" case as a no-op or warning.
func Write(dir string, c Config) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := Path(dir)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := c.Validate(); err != nil {
		return err
	}
	// Build the YAML by hand so we can include comments. The content
	// is small enough that hand-rolling beats yaml.v3 Marshal+annotate.
	body := fmt.Sprintf(`# clu project configuration.
#
# Committed to git alongside workflow templates. Local-only state
# (the SQLite database, WAL files) lives in this same directory but
# is gitignored.

# Prefix for newly-allocated issue IDs. Existing IDs are unaffected
# if you change this. Lowercase a-z + digits + dashes, ending with
# "-". Max 16 chars. Example: "acme-" → acme-a3f8.
id_prefix: %s

# Declared agents (Claude Code sessions or other clu-aware processes).
# Each name is the value to pass to ` + "`clu claim --agent <name>`" + `; capabilities
# are the cap:* labels they'll match for unassigned-lane work.
#
# Coordinators run ` + "`clu agent ls`" + ` to see who exists and who's currently
# live (heartbeating from a --wait/--watch loop).
#
# Uncomment + edit to define your team:
#
# agents:
#   code-reviewer:
#     description: "Reviews Go code for correctness and security"
#     capabilities: [go-review, security-review]
#   doc-writer:
#     description: "Writes README + docs/ updates"
#     capabilities: [docs]
`, c.IDPrefix)
	return os.WriteFile(path, []byte(body), 0o644)
}

