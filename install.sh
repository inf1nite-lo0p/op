#!/usr/bin/env sh
# op installer.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/inf1nite-lo0p/op/main/install.sh | sh
#
# Downloads the latest pre-built op-bin binary for your OS/arch from
# GitHub Releases and installs it to ~/.local/bin (override with
# INSTALL_DIR=/wherever/you/want sh ./install.sh). Then prints the
# one-line shell-init snippet for your rc file.
#
# No Go toolchain required.

set -eu

REPO="inf1nite-lo0p/op"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BIN_NAME="op-bin"

# ---------- pretty printing ----------------------------------------

if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  bold=$(printf '\033[1m')
  cyan=$(printf '\033[36m')
  green=$(printf '\033[32m')
  red=$(printf '\033[31m')
  dim=$(printf '\033[2m')
  reset=$(printf '\033[0m')
else
  bold=""; cyan=""; green=""; red=""; dim=""; reset=""
fi

info() { printf '%s%s%s %s\n' "$cyan" "→" "$reset" "$1"; }
ok()   { printf '%s%s%s %s\n' "$green" "✓" "$reset" "$1"; }
err()  { printf '%s%s%s %s\n' "$red" "✗" "$reset" "$1" >&2; }

# ---------- platform detection -------------------------------------

uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
  Linux)  os="linux"  ;;
  Darwin) os="darwin" ;;
  *)
    err "Unsupported OS: $uname_s"
    err "op currently ships pre-built binaries for Linux and macOS only."
    err "Build from source: https://github.com/${REPO}#install"
    exit 1
    ;;
esac

case "$uname_m" in
  x86_64|amd64) arch="x86_64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)
    err "Unsupported architecture: $uname_m"
    err "Build from source: https://github.com/${REPO}#install"
    exit 1
    ;;
esac

info "Detected ${bold}${os}/${arch}${reset}"

# ---------- find the latest release tag ---------------------------

# Use the JSON API but parse with grep+cut so we don't depend on jq.
api="https://api.github.com/repos/${REPO}/releases/latest"
info "Looking up latest release…"
tag=$(curl -fsSL "$api" 2>/dev/null \
  | grep '"tag_name"' \
  | head -n 1 \
  | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')

if [ -z "${tag:-}" ]; then
  err "Couldn't resolve the latest release of ${REPO}."
  err "Either there are no releases yet, or GitHub's API rate-limited you."
  err "Manual install: https://github.com/${REPO}/releases"
  exit 1
fi

version="${tag#v}"
asset="op_${version}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"

info "Downloading ${bold}${asset}${reset}"
printf '  %s%s%s\n' "$dim" "$url" "$reset"

# ---------- download + extract ------------------------------------

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t op-install)
trap 'rm -rf "$tmp"' EXIT

if ! curl -fsSL "$url" -o "$tmp/op.tar.gz"; then
  err "Download failed."
  err "URL: $url"
  exit 1
fi

if ! tar -xzf "$tmp/op.tar.gz" -C "$tmp"; then
  err "Tarball extraction failed."
  exit 1
fi

if [ ! -f "$tmp/$BIN_NAME" ]; then
  err "Tarball didn't contain $BIN_NAME — release artifact may be malformed."
  exit 1
fi

# ---------- install ------------------------------------------------

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
ok "Installed ${bold}${INSTALL_DIR}/${BIN_NAME}${reset}"

# ---------- shell-init suggestion ---------------------------------

# Best-effort detect the user's shell. SHELL env var is set by login;
# if it's not bash/zsh we just default to bash, which works in zsh too.
case "${SHELL:-}" in
  */zsh) shell="zsh"; rc="$HOME/.zshrc" ;;
  */bash) shell="bash"; rc="$HOME/.bashrc" ;;
  *) shell="bash"; rc="" ;;
esac

# Warn if the install dir isn't on PATH — easy to miss otherwise.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '\n%s%s%s %s%s%s isn'\''t on your PATH.\n' \
      "$dim" "(note)" "$reset" "$bold" "$INSTALL_DIR" "$reset"
    printf '%s\n' "  Add this to your rc file (typically the line just above op-bin's eval):"
    printf '%s    %sexport PATH="%s:$PATH"%s\n' "$dim" "$reset" "$INSTALL_DIR" "$reset"
    ;;
esac

cat <<EOF

${bold}Next:${reset} wire up the shell function by adding this to your rc file:

  ${bold}eval "\$($BIN_NAME shell-init $shell)"${reset}

EOF

if [ -n "$rc" ]; then
  cat <<EOF
Or run this once:

  ${bold}echo 'eval "\$($BIN_NAME shell-init $shell)"' >> $rc${reset}

Then start a new shell (or source $rc) and type ${bold}op${reset}.
EOF
else
  printf 'Then start a new shell and type %sop%s.\n' "$bold" "$reset"
fi
