#!/usr/bin/env bash
# Generate a realistic-looking set of fake git repos for the op demo.
#
# Doesn't fork the real `git` binary — op only reads .git/HEAD via
# its own parser, so we write the metadata directly. Each repo gets
# a HEAD ref + a controlled mtime so the picker's "last touched"
# column shows variety.
#
# Output: a self-contained directory tree under a *fake* $HOME so
# the picker renders paths as a clean `~/code/acme/api` instead of
# the absolute `/some/local/dir/.demo/projects/...` (which would
# leak the recorder's real username + path layout into the GIF).
#
#   ./assets/seed-demo.sh [TARGET_DIR]
#
# TARGET_DIR defaults to ./.demo (gitignored). Layout:
#
#   $TARGET/home/code/acme/api/      ← fake $HOME's project tree
#   $TARGET/home/personal/dotfiles/
#   $TARGET/home/playground/...
#   $TARGET/config/op/config.toml    ← XDG config (refers to ~/code, …)
#   $TARGET/cache/op/                ← XDG cache
#
# Recording / running scoped to it:
#   HOME=$TARGET/home \
#   XDG_CONFIG_HOME=$TARGET/config \
#   XDG_CACHE_HOME=$TARGET/cache \
#   op

set -euo pipefail

TARGET="${1:-$PWD/.demo}"
DEMO_HOME="$TARGET/home"

# Reset.
rm -rf "$TARGET"
mkdir -p "$TARGET/config/op" "$TARGET/cache/op" "$DEMO_HOME"

# Wall-clock in epoch seconds; we subtract offsets per repo to give
# realistic recency.
NOW=$(date +%s)

# set_mtime <file> <epoch_seconds> — portable across GNU and BSD touch.
set_mtime() {
  local file="$1" epoch="$2"
  if touch -d "@$epoch" "$file" 2>/dev/null; then return; fi
  local ts
  ts=$(date -r "$epoch" +%Y%m%d%H%M.%S 2>/dev/null) && touch -t "$ts" "$file"
}

# offset_secs <human> — accepts "1 hour", "3 days", "2 weeks". Pure
# bash arithmetic so we don't depend on `date -d "X ago"` (BSD lacks).
offset_secs() {
  local n unit
  read -r n unit <<<"$1"
  case "$unit" in
    second|seconds) echo $((n));;
    minute|minutes) echo $((n * 60));;
    hour|hours)     echo $((n * 3600));;
    day|days)       echo $((n * 86400));;
    week|weeks)     echo $((n * 86400 * 7));;
    month|months)   echo $((n * 86400 * 30));;
    *) echo "unknown unit: $unit" >&2; return 1;;
  esac
}

# make_repo <relative_path> <branch> <age>
# Path is relative to the fake $HOME.
make_repo() {
  local rel="$1" branch="$2" age="$3"
  local path="$DEMO_HOME/$rel"
  mkdir -p "$path/.git/worktrees"
  printf 'ref: refs/heads/%s\n' "$branch" >"$path/.git/HEAD"
  set_mtime "$path/.git/HEAD" $((NOW - $(offset_secs "$age")))
}

# make_worktree <main_relative> <wt_name> <branch> <age>
# Mirrors the .claude/worktrees/<name>/ layout.
make_worktree() {
  local main="$DEMO_HOME/$1" name="$2" branch="$3" age="$4"
  local wt_dir="$main/.claude/worktrees/$name"
  local wt_gitdir="$main/.git/worktrees/$name"
  mkdir -p "$wt_dir" "$wt_gitdir"
  printf 'ref: refs/heads/%s\n' "$branch" >"$wt_gitdir/HEAD"
  printf 'gitdir: %s\n' "$wt_gitdir" >"$wt_dir/.git"
  printf '%s/.git\n' "$wt_dir" >"$wt_gitdir/gitdir"
  set_mtime "$wt_gitdir/HEAD" $((NOW - $(offset_secs "$age")))
}

# ----- acme workspace ---------------------------------------------

make_repo     code/acme/api           main "2 hours"
make_worktree code/acme/api  feat-auth-jwt-rotation       feat-auth-jwt-rotation       "1 hour"
make_worktree code/acme/api  bugfix-login-redirect-loop   bugfix-login-redirect-loop   "4 hours"
make_worktree code/acme/api  feat-rate-limiter            feat-rate-limiter            "9 hours"

make_repo     code/acme/api-gateway   main "1 day"
make_repo     code/acme/api-docs      main "3 days"
make_repo     code/acme/web           main "6 hours"
make_worktree code/acme/web  feat-checkout-redesign       feat-checkout-redesign       "5 hours"

make_repo     code/acme/worker        main "2 days"
make_repo     code/acme/shared-types  main "5 days"
make_repo     code/acme/infra         main "1 week"

# ----- personal stuff ---------------------------------------------

make_repo     personal/dotfiles       main "2 weeks"
make_repo     personal/blog           main "3 days"
make_repo     personal/notes          main "5 days"

# ----- playground -------------------------------------------------

make_repo     playground/rust-async-channels  main "1 week"
make_repo     playground/tinygo-experiment    main "2 weeks"
make_repo     playground/htmx-todo            main "10 days"

# ----- op config (TOML) -------------------------------------------
#
# Roots are written as ~-relative so they look like a real user's
# config in `op config edit`. The fake-HOME setup at runtime makes
# them resolve to the seeded tree.

cat >"$TARGET/config/op/config.toml" <<'TOML'
roots = ["~/code", "~/personal", "~/playground"]
prune = ["node_modules", "vendor", "target", "dist", "build"]
vim_mode = false
TOML

# ----- bash setup snippet for the demo recording ------------------
#
# vhs's tape parser fights with `\$` inside `Type "…"`, so the demo
# tape sources this file instead of inlining the bash. Two things
# happen here:
#
#   1. `bind 'set show-mode-in-prompt off'` removes bash's readline
#      `@` indicator (default for emacs mode) that would otherwise
#      sit in front of every prompt in the GIF.
#   2. PROMPT_COMMAND rebuilds PS1 on every prompt so the shown cwd
#      is home-relative — we don't have to follow the picker with a
#      separate `pwd` line just to prove the cd happened.

cat >"$TARGET/promptrc" <<'EOF'
bind 'set show-mode-in-prompt off' 2>/dev/null
# `${PWD#$HOME}` strips the HOME prefix from PWD (giving "/code/acme/api"
# from "/some/path/home/code/acme/api"); prepending `~` reconstructs a
# clean home-relative cwd. The substitute-prefix form
# `${PWD/#$HOME/~}` would be tidier but doesn't fire reliably in this
# bash build.
PROMPT_COMMAND='PS1="~${PWD#$HOME} $ "'
EOF

# ----- pre-warm cache so the demo opens instantly ------------------
#
# Important: we have to set HOME for this so the cached paths are
# under the fake home and prettyPath collapses them to ~ at render
# time in the GIF.

op_bin="${OP_BIN:-./bin/op-bin}"
if [ -x "$op_bin" ]; then
  HOME="$DEMO_HOME" \
  XDG_CONFIG_HOME="$TARGET/config" \
  XDG_CACHE_HOME="$TARGET/cache" \
    "$op_bin" refresh >/dev/null 2>&1 || true
fi

# ----- summary ----------------------------------------------------

count=$(find "$DEMO_HOME" -type d -name '.git' | wc -l | tr -d ' ')
wt=$(find "$DEMO_HOME" -type f -name '.git' | wc -l | tr -d ' ')

cat <<EOF
✓ Demo projects ready
  fake home:   $DEMO_HOME
  main repos:  $count
  worktrees:   $wt

  Try it:
    HOME=$DEMO_HOME XDG_CONFIG_HOME=$TARGET/config XDG_CACHE_HOME=$TARGET/cache $op_bin
EOF
