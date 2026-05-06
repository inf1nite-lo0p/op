#!/usr/bin/env bash
# Generate a realistic-looking set of fake git repos for the op demo.
#
# Doesn't fork the real `git` binary — op only reads .git/HEAD via
# its own parser, so we write the metadata directly. Each repo gets
# a HEAD ref + a controlled mtime so the picker's "last touched"
# column shows variety.
#
# Output: a self-contained directory tree plus an op config pointing
# at it, ready to be picked up via XDG_{CONFIG,CACHE}_HOME env vars
# without polluting the user's real ~/.config/op/config.toml.
#
#   ./assets/seed-demo.sh [TARGET_DIR]
#
# TARGET_DIR defaults to ./.demo (gitignored). After this runs:
#   XDG_CONFIG_HOME=$TARGET/config XDG_CACHE_HOME=$TARGET/cache op
# scopes op entirely to the seeded projects.

set -euo pipefail

TARGET="${1:-$PWD/.demo}"
PROJECTS="$TARGET/projects"

# Reset.
rm -rf "$TARGET"
mkdir -p "$TARGET/config/op" "$TARGET/cache/op" "$PROJECTS"

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

# make_repo <path> <branch> <age>
make_repo() {
  local path="$1" branch="$2" age="$3"
  mkdir -p "$path/.git/worktrees"
  printf 'ref: refs/heads/%s\n' "$branch" >"$path/.git/HEAD"
  set_mtime "$path/.git/HEAD" $((NOW - $(offset_secs "$age")))
}

# make_worktree <main_repo> <wt_name> <branch> <age>
# Mirrors the .claude/worktrees/<name>/ layout so worktrees show up
# nested under their main, the same shape op was built around.
make_worktree() {
  local main="$1" name="$2" branch="$3" age="$4"
  local wt_dir="$main/.claude/worktrees/$name"
  local wt_gitdir="$main/.git/worktrees/$name"
  mkdir -p "$wt_dir" "$wt_gitdir"
  printf 'ref: refs/heads/%s\n' "$branch" >"$wt_gitdir/HEAD"
  printf 'gitdir: %s\n' "$wt_gitdir" >"$wt_dir/.git"
  printf '%s/.git\n' "$wt_dir" >"$wt_gitdir/gitdir"
  set_mtime "$wt_gitdir/HEAD" $((NOW - $(offset_secs "$age")))
}

# ----- acme workspace ---------------------------------------------

make_repo     "$PROJECTS/acme/api"           main "2 hours"
make_worktree "$PROJECTS/acme/api"           feat-auth-jwt-rotation       feat-auth-jwt-rotation       "1 hour"
make_worktree "$PROJECTS/acme/api"           bugfix-login-redirect-loop   bugfix-login-redirect-loop   "4 hours"
make_worktree "$PROJECTS/acme/api"           feat-rate-limiter            feat-rate-limiter            "9 hours"

make_repo     "$PROJECTS/acme/api-gateway"   main "1 day"
make_repo     "$PROJECTS/acme/api-docs"      main "3 days"
make_repo     "$PROJECTS/acme/web"           main "6 hours"
make_worktree "$PROJECTS/acme/web"           feat-checkout-redesign       feat-checkout-redesign       "5 hours"

make_repo     "$PROJECTS/acme/worker"        main          "2 days"
make_repo     "$PROJECTS/acme/shared-types"  main          "5 days"
make_repo     "$PROJECTS/acme/infra"         main          "1 week"

# ----- personal stuff ---------------------------------------------

make_repo     "$PROJECTS/personal/dotfiles"  main          "2 weeks"
make_repo     "$PROJECTS/personal/blog"      main          "3 days"
make_repo     "$PROJECTS/personal/notes"     main          "5 days"

# ----- playground -------------------------------------------------

make_repo     "$PROJECTS/playground/rust-async-channels"  main "1 week"
make_repo     "$PROJECTS/playground/tinygo-experiment"    main "2 weeks"
make_repo     "$PROJECTS/playground/htmx-todo"            main "10 days"

# ----- op config (TOML) -------------------------------------------

cat >"$TARGET/config/op/config.toml" <<TOML
roots = ["$PROJECTS"]
prune = ["node_modules", "vendor", "target", "dist", "build"]
vim_mode = false
TOML

# ----- pre-warm cache so the demo opens instantly ------------------
#
# Walks the seeded tree once via the same scanner the picker uses,
# so the first `op` keystroke in the GIF lands on the picker view
# instead of the "first-time scan" banner.

op_bin="${OP_BIN:-./bin/op-bin}"
if [ -x "$op_bin" ]; then
  XDG_CONFIG_HOME="$TARGET/config" \
  XDG_CACHE_HOME="$TARGET/cache" \
    "$op_bin" refresh >/dev/null 2>&1 || true
fi

# ----- summary ----------------------------------------------------

count=$(find "$PROJECTS" -type d -name '.git' | wc -l | tr -d ' ')
wt=$(find "$PROJECTS" -type f -name '.git' | wc -l | tr -d ' ')

cat <<EOF
✓ Demo projects ready
  location:    $PROJECTS
  main repos:  $count
  worktrees:   $wt

  Try it:
    XDG_CONFIG_HOME=$TARGET/config XDG_CACHE_HOME=$TARGET/cache $op_bin
EOF
