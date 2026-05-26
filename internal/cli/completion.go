package cli

import (
	"fmt"
	"strings"
)

// completionCmds lists the top-level subcommands for shell completion.
// Keep in sync with the CLI struct in cli.go when adding commands.
var completionCmds = []string{
	"init", "create", "list", "ready", "blocked", "show", "claim", "close",
	"update", "dep", "label", "defer", "undefer",
	"count", "stats", "reopen",
	"assign", "priority", "tag", "link",
	"export", "import",
	"info", "statuses", "types", "doctor",
	"describe", "note", "comment", "kv",
	"cron", "agent",
	"run", "template", "checkpoint", "approve",
	"version", "completion",
}

type CompletionCmd struct {
	Shell string `arg:"" enum:"bash,zsh,fish" help:"Target shell (bash, zsh, or fish)."`
}

func (c *CompletionCmd) Run(r *runCtx) error {
	cmds := strings.Join(completionCmds, " ")
	switch c.Shell {
	case "bash":
		fmt.Fprintf(r.stdout, `# bash completion for clu. Source this file or add to ~/.bashrc:
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
		fmt.Fprintf(r.stdout, `# zsh completion for clu. Source this file or add to ~/.zshrc:
#   source <(clu completion zsh)
#compdef clu
_clu() {
    local commands=( %s )
    _arguments "1: :($commands)" "*::arg:->args"
}
compdef _clu clu
`, cmds)
	case "fish":
		fmt.Fprintln(r.stdout, "# fish completion for clu. Save to ~/.config/fish/completions/clu.fish:")
		fmt.Fprintln(r.stdout, "#   clu completion fish > ~/.config/fish/completions/clu.fish")
		for _, cmd := range completionCmds {
			fmt.Fprintf(r.stdout, "complete -c clu -n '__fish_use_subcommand' -a %s\n", cmd)
		}
	}
	return nil
}
