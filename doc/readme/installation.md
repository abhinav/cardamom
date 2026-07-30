# Installation

Install the Cardamom plugin for your agent host.
The plugin supplies the Cardamom skill
and makes the matching `card` CLI available when the agent needs it.

## Claude Code

1. Add the Cardamom repository marketplace.

    ```bash
    claude plugin marketplace add abhinav/cardamom
    ```

2. Install the plugin.

    ```bash
    claude plugin install cardamom@cardamom
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

## First use

Start Claude Code or Codex in the project you want to coordinate,
then ask the agent:

```text
Use the Cardamom skill to plan and coordinate this work.
```

The plugin uses an existing `card` executable from `PATH` when one is available.
Otherwise,
its packaged launcher downloads and verifies the Cardamom release
that matches the plugin version.
On macOS and Linux,
the launcher caches that executable at
`~/.cache/cardamom-skill/versions/<version>/cardamom`.
On Windows,
it uses
`%LOCALAPPDATA%\cardamom-skill\versions\<version>\cardamom.exe`.
Updating the plugin selects the matching release and cache directory.

## Updates

- **Claude Code**

    ```bash
    claude plugin marketplace update cardamom
    claude plugin update cardamom@cardamom
    ```

- **Codex**

    ```bash
    codex plugin marketplace upgrade cardamom
    codex plugin add cardamom@cardamom
    ```

## Direct CLI installation

Install the public `card` command directly
when you want to use Cardamom from your shell
or need a recovery path that does not use the plugin launcher.

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

- **Source checkout**

    Clone the repository,
    set up [Mise](https://mise.jdx.dev/),
    and run:

    ```bash
    mise run build
    cp bin/card "$HOME/.local/bin"
    ```
