# Installation

Install Cardamom for Claude Code, Codex, or as a standalone command.

## Claude Code

1. Add the Cardamom repository marketplace.

    ```bash
    claude plugin marketplace add abhinav/cardamom
    ```

2. Install the plugin.

    ```bash
    claude plugin install cardamom@cardamom
    ```

To update the plugin:

```bash
claude plugin marketplace update cardamom
claude plugin update cardamom@cardamom
```

## Codex

1. Add the Cardamom repository marketplace.

    ```bash
    codex plugin marketplace add abhinav/cardamom
    ```

2. Install the plugin.

    ```bash
    codex plugin add cardamom@cardamom
    ```

To update the plugin:

```bash
codex plugin marketplace upgrade cardamom
codex plugin add cardamom@cardamom
```

## Standalone command

Install the public `card` command directly to use Cardamom from your shell
or with another agent host.

- **Homebrew** or **Linuxbrew**

    ```bash
    brew install --cask abhinav/tap/cardamom
    ```

- **GitHub Releases**

    Pre-built binaries are available on the
    [GitHub Releases](https://github.com/abhinav/cardamom/releases) page.

    ```bash
    mkdir -p "$HOME/.local/bin"
    curl -fsSL "https://github.com/abhinav/cardamom/releases/latest/download/cardamom.$(uname -s)-$(uname -m).tar.gz" |
      tar -xz -C "$HOME/.local/bin" card
    ```

- **Install from source**

    To build and install Cardamom from source, clone the repository,
    set up [Mise](https://mise.jdx.dev/), and run:

    ```bash
    mise run build
    cp bin/card "$HOME/.local/bin"
    ```

After installing `card`,
install its embedded Cardamom skill into an agent host's parent skills
directory:

```bash
card skill install ~/.agents/skills
```

The command writes the skill to `~/.agents/skills/cardamom`.
An identical destination is left unchanged.
If the destination differs,
inspect it before passing `--force` to replace the complete directory.

### (Optional) Set up the hook

Plugin installations include lifecycle hooks automatically.
For a standalone Claude Code installation,
merge the following configuration into `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "card hook context",
            "timeout": 5
          }
        ]
      }
    ],
    "SubagentStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "card hook context",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

For a standalone Codex installation,
merge the following configuration into `~/.codex/hooks.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "card hook context",
            "commandWindows": "card hook context",
            "timeout": 5
          }
        ]
      }
    ],
    "SubagentStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "card hook context",
            "commandWindows": "card hook context",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

The hook adds Cardamom skill guidance only when the current checkout has a
local Cardamom entry or board binding.
It remains silent in other directories.
