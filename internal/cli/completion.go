package cli

import (
	"fmt"
	"strings"
)

// completionCmds lists the top-level subcommands for shell completion.
// Keep in sync with the CLI struct in cli.go when adding commands.
var completionCmds = []string{
	"init", "create", "list", "ls", "ready", "blocked", "show", "claim", "close",
	"cancel",
	"update", "dep", "label", "defer", "undefer",
	"history", "log",
	"count", "stats", "reopen",
	"assign", "priority", "tag", "link",
	"export", "import", "batch", "sql",
	"info", "statuses", "types", "doctor",
	"describe", "note", "comment", "kv",
	"cron", "agent", "brief",
	"lock", "unlock", "locks",
	"ping", "inbox", "worktree",
	"run", "template", "checkpoint", "approve",
	"http", "web",
	"version", "completion",
}

type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish" help:"Target shell (bash, zsh, or fish)."`
}

func (c *CompletionCmd) Run(r *runCtx) error {
	script := completionScript(c.Shell)
	// Under --json the script is a string field so the output is still
	// exactly one JSON value (the raw script isn't valid JSON on its own).
	if r.json {
		return r.emitJSON(map[string]any{"shell": c.Shell, "script": script})
	}
	fmt.Fprint(r.stdout, script)
	return nil
}

// completionScript renders the completion script for the given shell.
func completionScript(shell string) string {
	cmds := strings.Join(completionCmds, " ")
	switch shell {
	case "bash":
		return fmt.Sprintf(`# bash completion for clu. Source this file or add to ~/.bashrc:
#   source <(clu completion bash)
_clu_completions() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "%s" -- "$cur") )
    fi
}
complete -F _clu_completions clu
`, cmds)
	case "zsh":
		return fmt.Sprintf(`# zsh completion for clu. Source this file or add to ~/.zshrc:
#   source <(clu completion zsh)
#compdef clu
_clu() {
    local commands=( %s )
    _arguments "1: :($commands)" "*::arg:->args"
}
compdef _clu clu
`, cmds)
	case "fish":
		var b strings.Builder
		fmt.Fprintln(&b, "# fish completion for clu. Save to ~/.config/fish/completions/clu.fish:")
		fmt.Fprintln(&b, "#   clu completion fish > ~/.config/fish/completions/clu.fish")
		for _, cmd := range completionCmds {
			fmt.Fprintf(&b, "complete -c clu -n '__fish_use_subcommand' -a %s\n", cmd)
		}
		return b.String()
	}
	return ""
}
