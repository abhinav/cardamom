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
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the parsed shape of <dir>/config.yaml. New top-level fields
// MUST round-trip through yaml.v3 with strict unmarshalling (unknown
// keys are an error — see Load) so typos surface immediately.
type Config struct {
	IDPrefix string           `yaml:"id_prefix"`
	Agents   map[string]Agent `yaml:"agents,omitempty"`
	Worktree Worktree         `yaml:"worktree,omitempty"`
}

// Worktree drives `clu worktree add --bootstrap` and `clu worktree
// bootstrap`. Each new git worktree starts as a fresh filesystem tree —
// gitignored files (.env, secrets, install caches) and per-checkout
// scripts (pnpm install, db migrate) don't come along.
//
// Paths in Copy are relative to the *main* worktree (auto-detected via
// git) and land at the same relative path inside the new worktree.
// Commands run from inside the new worktree with the user's $PATH +
// $HOME and inherit stdin, so prompts work.
//
// Dir is the folder inside the main worktree where bare-name worktrees
// land — `clu worktree add foo` creates `<main>/<Dir>/foo`. Default
// `.worktrees`. Auto-added to .gitignore on first use.
type Worktree struct {
	Dir      string   `yaml:"dir,omitempty"`      // default location for bare-name worktrees (default ".worktrees")
	Copy     []string `yaml:"copy,omitempty"`     // files to copy from main worktree → new worktree
	Commands []string `yaml:"commands,omitempty"` // shell snippets run inside the new worktree
}

// DefaultWorktreeDir is the fallback folder name used when
// `worktree.dir` is unset.
const DefaultWorktreeDir = ".worktrees"

// Agent is the declarative side of an agent: who they are, what they can
// do, and how to launch them. Committed to git. The live side (heartbeat,
// pid, host) lives in the active_agents table, populated by --wait/--watch
// loops and by `clu agent start`.
//
// The launch fields drive `clu agent start <name>`:
//   - Command is the executable (e.g. "claude"). Required to start.
//   - Prompts are files under <dir>/agents/<name>/ passed to the command,
//     each preceded by PromptFlag. Empty → every *.md in that folder.
//   - PromptFlag is how each prompt path is passed (default
//     "--append-system-prompt" when Command is "claude"; otherwise
//     required when Prompts are present).
//   - Args are extra static arguments appended after the prompts.
//   - StartupPrompt, if set, is passed as the command's trailing
//     positional — the agent's first message. Use it to seed startup
//     steps the agent should run itself (e.g. "check `clu inbox -a
//     developer`, then claim ready work").
type Agent struct {
	Description   string   `yaml:"description,omitempty"`
	Capabilities  []string `yaml:"capabilities,omitempty"`
	Command       string   `yaml:"command,omitempty"`
	PromptFlag    string   `yaml:"prompt_flag,omitempty"`
	Prompts       []string `yaml:"prompts,omitempty"`
	Args          []string `yaml:"args,omitempty"`
	StartupPrompt string   `yaml:"startup_prompt,omitempty"`
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

// ValidAgentOrCapName reports whether s matches the agent / capability
// naming rule. Exported so the CLI surface enforces the same rule that
// config.yaml does — previously `clu create --capability foo:bar`
// silently produced labels no declared agent could match.
func ValidAgentOrCapName(s string) bool {
	return agentNameRe.MatchString(s)
}

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
		for _, p := range a.Prompts {
			if p == "" {
				return fmt.Errorf("agent %q: prompts: empty entry", name)
			}
			if filepath.IsAbs(p) {
				return fmt.Errorf("agent %q: prompt %q: must be relative to agents/%s/, not absolute", name, p, name)
			}
			if strings.Contains(p, "..") {
				return fmt.Errorf("agent %q: prompt %q: must not contain '..'", name, p)
			}
		}
	}
	if c.Worktree.Dir != "" {
		if filepath.IsAbs(c.Worktree.Dir) {
			return fmt.Errorf("worktree.dir %q: must be relative to the main worktree", c.Worktree.Dir)
		}
		if strings.Contains(c.Worktree.Dir, "..") {
			return fmt.Errorf("worktree.dir %q: must not contain '..'", c.Worktree.Dir)
		}
	}
	for _, p := range c.Worktree.Copy {
		if p == "" {
			return errors.New("worktree.copy: empty entry")
		}
		if filepath.IsAbs(p) {
			return fmt.Errorf("worktree.copy %q: must be relative to the worktree root, not absolute", p)
		}
		if strings.Contains(p, "..") {
			return fmt.Errorf("worktree.copy %q: must not contain '..'", p)
		}
	}
	for _, cmd := range c.Worktree.Commands {
		if cmd == "" {
			return errors.New("worktree.commands: empty entry")
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
# Each name is the value to pass to `+"`clu claim --agent <name>`"+`; capabilities
# are the cap:* labels they'll match for unassigned-lane work.
#
# Coordinators run `+"`clu agent ls`"+` to see who exists and who's currently
# live (heartbeating from a --wait/--watch loop).
#
# Uncomment + edit to define your team:
#
# agents:
#   code-reviewer:
#     description: "Reviews Go code for correctness and security"
#     capabilities: [go-review, security-review]
#     # Launch with: clu agent start code-reviewer
#     # command is the executable; prompts are files under
#     # .clu/agents/code-reviewer/ passed to it (claude defaults
#     # prompt_flag to --append-system-prompt). Omit prompts to use every
#     # *.md in that folder. Run with --print to see the command first.
#     #
#     # Shared base: any *.md in .clu/agents/_shared/ is prepended to
#     # every agent (e.g. a common AGENTS.md / AUTONOMY.md) so the shared
#     # contract lives in one place. Per-agent prompts come after it.
#     #
#     # startup_prompt (optional) is the agent's first message, passed as
#     # a trailing positional. Use it for startup steps the agent runs
#     # itself, e.g.:
#     #   startup_prompt: |
#     #     Check your inbox (clu inbox -a code-reviewer), then claim work.
#     command: claude
#     prompts: [AGENTS.md, SOUL.md]
#   doc-writer:
#     description: "Writes README + docs/ updates"
#     capabilities: [docs]

# Bootstrap recipe for new git worktrees. Run via
#   clu worktree add <path> [<ref>] --bootstrap
#   clu worktree bootstrap <path>
#
# Files in `+"`copy`"+` are pulled from the main worktree (auto-detected via
# git) into the same relative path inside the new worktree — for
# gitignored secrets / .env / install state that fresh checkouts lack.
# Commands run inside the new worktree, fail-fast on the first non-zero
# exit.
#
# worktree:
#   dir: .worktrees       # where bare-name worktrees land (default)
#   copy:
#     - .env
#     - apps/api/.env
#   commands:
#     - pnpm install
#     - pnpm db:migrate
`, c.IDPrefix)
	return os.WriteFile(path, []byte(body), 0o644)
}
