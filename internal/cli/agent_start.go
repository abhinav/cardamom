package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Rovak/agents-clu/internal/config"
	"github.com/Rovak/agents-clu/internal/store"
)

// sharedPromptDir is the conventional folder under <dir>/agents/ whose
// *.md files are prepended to every agent's prompts. It lets the common
// contract (e.g. AGENTS.md, AUTONOMY.md) live in one place instead of
// being copied into each agent folder. The leading underscore keeps it
// from ever colliding with an agent name (those must start with a
// letter — see config.ValidAgentOrCapName).
const sharedPromptDir = "_shared"

// AgentStartCmd launches a declared agent: it assembles a command from
// the agent's config.yaml launch spec and execs it in the foreground,
// inheriting the terminal. While the child runs, a heartbeat keeps the
// agent visible as live in `clu agent ls`; the row is cleared on exit.
//
// Prompt files are layered: the shared base (<dir>/agents/_shared/*.md,
// sorted) comes first, then the agent's own prompts, so a persona refines
// the shared contract rather than restating it. An optional
// startup_prompt is passed as the command's trailing positional — the
// agent's first message.
//
// This is a thin launcher, not a supervisor — no daemonizing, no
// restart-on-crash. Use --print to see the assembled command without
// running it (handy for wiring clu into your own launcher).
type AgentStartCmd struct {
	Name  string   `arg:"" help:"Agent name (declared in config.yaml)."`
	Print bool     `name:"print" aliases:"dry-run" help:"Print the assembled command and exit without launching."`
	Rest  []string `arg:"" optional:"" name:"args" passthrough:"" help:"Extra arguments forwarded to the agent command."`
}

func (c *AgentStartCmd) Run(r *runCtx) error {
	cfg, err := config.Load(r.dir)
	if err != nil {
		return err
	}
	agent, ok := cfg.Agents[c.Name]
	if !ok {
		return fmt.Errorf("agent %q is not declared in config.yaml", c.Name)
	}
	if agent.Command == "" {
		return fmt.Errorf("agent %q has no `command` set in config.yaml (e.g. command: claude)", c.Name)
	}

	// Assemble the prompt files as full paths, shared layer first.
	agentsDir := filepath.Join(r.dir, "agents")
	promptDir := filepath.Join(agentsDir, c.Name)
	var promptPaths []string

	// Shared base: every *.md in <dir>/agents/_shared/, sorted, applied
	// to every agent before its own prompts.
	sharedMatches, _ := filepath.Glob(filepath.Join(agentsDir, sharedPromptDir, "*.md"))
	sort.Strings(sharedMatches)
	promptPaths = append(promptPaths, sharedMatches...)

	// Per-agent layer: an explicit prompts: list (relative to the agent
	// folder), or every *.md in it when unset.
	if len(agent.Prompts) > 0 {
		for _, p := range agent.Prompts {
			full := filepath.Join(promptDir, p)
			if _, err := os.Stat(full); err != nil {
				return fmt.Errorf("agent %q: prompt file not found: %s", c.Name, full)
			}
			promptPaths = append(promptPaths, full)
		}
	} else {
		ownMatches, _ := filepath.Glob(filepath.Join(promptDir, "*.md"))
		sort.Strings(ownMatches)
		promptPaths = append(promptPaths, ownMatches...)
	}

	// How each prompt is passed. claude is the common case, so default
	// its flag; any other command must declare prompt_flag if it has
	// prompts, since there's no portable convention.
	promptFlag := agent.PromptFlag
	if promptFlag == "" && len(promptPaths) > 0 {
		if filepath.Base(agent.Command) == "claude" {
			promptFlag = "--append-system-prompt"
		} else {
			return fmt.Errorf("agent %q: prompts are set but prompt_flag is not — declare how %q takes a prompt file (e.g. prompt_flag: --system-prompt)", c.Name, agent.Command)
		}
	}

	argv := []string{agent.Command}
	for _, full := range promptPaths {
		argv = append(argv, promptFlag, full)
	}
	argv = append(argv, agent.Args...)
	// A leading "--" is the flags/args separator, not an argument to
	// forward — drop one if present so `start name -- foo` passes `foo`.
	rest := c.Rest
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	argv = append(argv, rest...)
	// The startup prompt is the agent's first message — a trailing
	// positional (claude and codex both take one). Last so any flags in
	// Args/rest precede it.
	if agent.StartupPrompt != "" {
		argv = append(argv, agent.StartupPrompt)
	}

	if c.Print {
		if r.json {
			return r.emitJSON(map[string]any{"command": agent.Command, "argv": argv})
		}
		fmt.Fprintln(r.stdout, shellJoin(argv))
		return nil
	}

	caps := resolveAgent(r.dir, c.Name)
	return withStore(r, func(s *store.Store) error {
		return runAgentProcess(r, s, c.Name, caps, argv)
	})
}

// runAgentProcess execs argv in the foreground with inherited stdio,
// keeping the agent's heartbeat fresh until the child exits.
//
// Signal handling: the child runs in clu's process group, so the
// terminal delivers Ctrl+C (SIGINT) straight to it — claude/codex get to
// handle their own interrupt (e.g. cancel the current turn) rather than
// being hard-killed. We deliberately use exec.Command (not
// CommandContext) so clu never SIGKILLs the child on its own signal, and
// clu ignores SIGINT/SIGQUIT while the child is foreground so it doesn't
// tear itself down out from under the child. The child owns the terminal
// until it exits.
func runAgentProcess(r *runCtx, s *store.Store, name string, caps []string, argv []string) error {
	cleanup, err := startHeartbeat(s, name, caps)
	if err != nil {
		return err
	}
	defer cleanup()

	// Advance last_seen periodically so `agent ls` shows the session as
	// live for its whole lifetime, not just the first 30s.
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				heartbeatTick(s, name, caps)
			}
		}
	}()

	r.notice("starting %s: %s\n", name, argv[0])
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	// Ignore SIGINT/SIGQUIT in clu now that the child has its own copy
	// (inherited the default disposition at exec time). Set after Start
	// so the child isn't launched with SIG_IGN already in place.
	signal.Ignore(syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Reset(syscall.SIGINT, syscall.SIGQUIT)

	werr := cmd.Wait()
	// A child terminated by a signal (e.g. the user killing it) isn't a
	// clu error. A non-zero exit is surfaced so scripts see the failure.
	var exit *exec.ExitError
	if errors.As(werr, &exit) {
		if exit.Exited() {
			if code := exit.ExitCode(); code != 0 {
				return fmt.Errorf("%s exited with status %d", name, code)
			}
		}
		return nil
	}
	return werr
}

// shellSafe matches tokens that need no quoting in a copy-pasteable line.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellJoin renders argv as a single shell-pasteable line, single-quoting
// any token that isn't obviously safe.
func shellJoin(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && shellSafe.MatchString(a) {
			parts[i] = a
		} else {
			parts[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(parts, " ")
}
