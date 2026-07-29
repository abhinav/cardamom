# Installation

Use one of the following options to install Cardamom on your system.

- **Homebrew** or **Linuxbrew**:

    Install a pre-built binary of Cardamom from the official Homebrew tap:

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
