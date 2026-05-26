#!/usr/bin/env bash
# clu install script.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Rovak/agents-clu/main/install.sh | bash
#
# Detects OS + arch, downloads the latest matching release archive from
# https://github.com/Rovak/agents-clu/releases, extracts it, and installs
# the `clu` binary to one of:
#
#   1. $CLU_INSTALL_DIR  if set
#   2. /usr/local/bin    if writable (or sudo will be tried)
#   3. $HOME/.local/bin  fallback
#
# Optional environment variables:
#
#   CLU_VERSION         tag to install (default: latest release)
#   CLU_INSTALL_DIR     where to put the binary (default: see above)
#   CLU_REPO            owner/name override (default: Rovak/agents-clu)
#
# After install, prints a PATH-setup hint if the chosen dir isn't on PATH.

set -euo pipefail

CLU_REPO="${CLU_REPO:-Rovak/agents-clu}"
CLU_VERSION="${CLU_VERSION:-}"
CLU_INSTALL_DIR="${CLU_INSTALL_DIR:-}"

# Pretty output. Falls back gracefully when stdout isn't a terminal.
if [ -t 1 ]; then
    bold=$'\033[1m'
    cyan=$'\033[1;36m'
    yellow=$'\033[1;33m'
    red=$'\033[1;31m'
    reset=$'\033[0m'
else
    bold='' cyan='' yellow='' red='' reset=''
fi

log()  { printf '%s==>%s %s\n' "$cyan" "$reset" "$*"; }
warn() { printf '%swarn:%s %s\n' "$yellow" "$reset" "$*" >&2; }
die()  { printf '%serror:%s %s\n' "$red" "$reset" "$*" >&2; exit 1; }

# --- detect platform ----------------------------------------------------

detect_os() {
    case "$(uname -s)" in
        Linux)  echo linux ;;
        Darwin) echo darwin ;;
        *)      die "unsupported OS: $(uname -s) (clu publishes linux + darwin only)" ;;
    esac
}

detect_arch() {
    # goreleaser uses amd64 / arm64. Map uname's varied spellings to that.
    case "$(uname -m)" in
        x86_64 | amd64)         echo amd64 ;;
        aarch64 | arm64)        echo arm64 ;;
        *) die "unsupported arch: $(uname -m) (clu publishes amd64 + arm64 only)" ;;
    esac
}

# --- pick install dir ---------------------------------------------------

# Prefer an existing dir on PATH. Otherwise default to ~/.local/bin so we
# don't sudo unless the user explicitly opts in.
pick_install_dir() {
    if [ -n "$CLU_INSTALL_DIR" ]; then
        echo "$CLU_INSTALL_DIR"
        return
    fi
    # If /usr/local/bin is writable without sudo, use it.
    if [ -w /usr/local/bin ]; then
        echo /usr/local/bin
        return
    fi
    echo "$HOME/.local/bin"
}

# --- find the right asset name -----------------------------------------

# The release archive name without the version segment. We resolve the
# real name (and download URL) from the GitHub API rather than guessing
# strings so a future project_name rename doesn't break old installs.
fetch_release_json() {
    local url
    if [ -z "$CLU_VERSION" ]; then
        url="https://api.github.com/repos/${CLU_REPO}/releases/latest"
    else
        url="https://api.github.com/repos/${CLU_REPO}/releases/tags/${CLU_VERSION}"
    fi
    curl -fsSL "$url"
}

# Pull the asset URL matching <os>_<arch>.tar.gz out of the API JSON.
# Uses sed/grep so we don't require jq.
find_asset_url() {
    local os="$1" arch="$2"
    local json
    json="$(fetch_release_json)" || die "failed to fetch release info from GitHub"
    printf '%s\n' "$json" \
        | grep -o '"browser_download_url": *"[^"]*"' \
        | sed 's/.*"\([^"]*\)"$/\1/' \
        | grep -E "_${os}_${arch}\.tar\.gz$" \
        | head -1
}

# --- main ---------------------------------------------------------------

main() {
    local os arch url install_dir tmp archive bin_name="clu" used_sudo=""

    os="$(detect_os)"
    arch="$(detect_arch)"
    install_dir="$(pick_install_dir)"

    log "platform:   ${bold}${os}/${arch}${reset}"
    log "install to: ${bold}${install_dir}${reset}"

    url="$(find_asset_url "$os" "$arch")"
    [ -n "$url" ] || die "no release asset matching ${os}_${arch}.tar.gz on $CLU_REPO. Set CLU_VERSION to an existing tag, or check https://github.com/${CLU_REPO}/releases"

    log "downloading: ${url}"

    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    archive="$tmp/clu.tar.gz"
    curl -fsSL "$url" -o "$archive"
    tar -xzf "$archive" -C "$tmp"

    [ -f "$tmp/$bin_name" ] || die "${bin_name} binary not present in archive (${url}); something is wrong with the release"
    chmod +x "$tmp/$bin_name"

    # Install into the chosen dir. If the dir doesn't exist we'll try to
    # create it. If writes fail and we're heading for a system dir, try
    # sudo once; otherwise bail with a clear hint.
    mkdir -p "$install_dir" 2>/dev/null || true
    if ! mv "$tmp/$bin_name" "$install_dir/$bin_name" 2>/dev/null; then
        case "$install_dir" in
            /usr/local/bin | /usr/bin | /opt/*)
                log "elevating with sudo for $install_dir"
                sudo mv "$tmp/$bin_name" "$install_dir/$bin_name"
                sudo chmod +x "$install_dir/$bin_name"
                used_sudo=1
                ;;
            *)
                die "cannot write to $install_dir. Re-run with CLU_INSTALL_DIR=/path/you/can/write or pick a different dir."
                ;;
        esac
    fi

    log "installed   ${bold}${install_dir}/${bin_name}${reset}"
    [ -n "$used_sudo" ] && log "(used sudo)"

    # PATH hint.
    case ":$PATH:" in
        *":$install_dir:"*) ;;
        *)
            warn "$install_dir is not on your PATH yet. Add this to your shell rc:"
            printf '    %sexport PATH="%s:$PATH"%s\n' "$bold" "$install_dir" "$reset"
            ;;
    esac

    # Final smoke. We try to invoke clu from the install location so we
    # confirm chmod and the architecture choice were right.
    if "$install_dir/$bin_name" version >/dev/null 2>&1; then
        log "$("$install_dir/$bin_name" version 2>&1 | head -1)"
    else
        warn "installed binary couldn't run \`clu version\` — please check $install_dir/$bin_name"
    fi

    cat <<NEXT

${bold}Next steps:${reset}
  ${cyan}clu init${reset}                     # initialise .clu/ in your project
  ${cyan}clu brief${reset}                    # print the agent-facing operational guide

For the web UI, clone the repo and run ${cyan}make install${reset} (requires pnpm).
NEXT
}

main "$@"
