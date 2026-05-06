# CLAUDE.md

This file gives Claude Code (and future you) the context to work
on this repo without re-reading the whole codebase.

## What this is

A fast TUI in Go for switching to local git projects (and worktrees)
from a fresh shell. End-user install:

```sh
curl -fsSL https://raw.githubusercontent.com/inf1nite-lo0p/op/main/install.sh | sh
```

Internally there are two pieces:

- `op-bin` — the Go binary (cmd/op-bin), built around bubbletea.
- `op` — a shell function that wraps the binary so a chosen path
  can `cd` the parent shell. The shim text lives in
  `internal/shellinit/op.bash` and is embedded into the binary,
  emitted by `op-bin shell-init <bash|zsh>`.

## Package layout

```
cmd/op-bin/main.go         entry, subcommand dispatch
internal/
├── cache/                 versioned JSON cache, atomic write
├── config/                TOML loader + Defaults + Set/Get/Keys
├── firstrun/              one-shot bubbletea form for first launch
├── gitmeta/               parse .git dirs/files (no git subprocess)
├── scanner/               parallel walker + prune logic + OnFound
├── shellinit/             go:embed of op.bash + Script(shell)
└── tui/
    ├── tui.go             Run, model, Update, View, applyFilter
    ├── delegate.go        per-row rendering (flex layout)
    ├── rank.go            tier-based search ranking + recency
    ├── vim.go             insert/normal-mode handlers + motions
    ├── styles.go          all lipgloss styles + initStyles
    ├── format.go          humanAgo, prettyPath, keyHelp
    └── tty.go             /dev/tty open helper
.goreleaser.yml            release build matrix (linux/darwin × amd64/arm64)
.github/workflows/
├── ci.yml                 vet + gofmt + build + test on push
└── release.yml            goreleaser on tag push (v*)
install.sh                 curl|sh installer for end users
```

## Day-to-day commands

```sh
just            # list recipes
just check      # vet + gofmt + tests (CI runs the same)
just install    # build + install to ~/.local/bin (dev path)
```

After editing the shell shim at `internal/shellinit/op.bash`, the
binary needs a rebuild — it's `go:embed`ed, not read at runtime.

## Releasing a new version

The release workflow in `.github/workflows/release.yml` runs
GoReleaser on every `v*` tag push. It builds for linux/darwin ×
amd64/arm64 and publishes a GitHub Release with tarballs +
`checksums.txt`. The `install.sh` script reads `/releases/latest`
from the GitHub API, so the curl|sh one-liner picks up new
versions automatically.

To ship a release:

```sh
# 1. Make sure main is green
just check

# 2. Pick the next version (semver)
git tag v0.2.0 -m "Brief release summary"

# 3. Push the tag — fires the workflow
git push origin v0.2.0

# 4. (Optional) Watch it run
gh run watch --repo inf1nite-lo0p/op --exit-status
gh release view v0.2.0 --repo inf1nite-lo0p/op
```

Version bumping convention:

- `v0.x.0` — new features or notable behaviour changes
- `v0.x.y` — bug fixes or doc-only changes
- `v1.0.0` — first release we'd consider stable for general use

Don't tag pre-1.0 releases as breaking-by-default; they're, but
keep changelogs honest in the tag message — GoReleaser's auto-
changelog also captures every commit since the last tag.

## Architectural rules

These were learned the hard way during the initial build, encoded
as tests where possible. Don't break them without good reason.

- **Cold launch reads cache and renders. Nothing else.** No
  filesystem walks, no git subprocess, no network on the cold
  path. The rescan goroutine fires after the picker is on screen.
- **No `git` binary on the hot path.** `gitmeta` parses
  `.git/HEAD` directly. Forking git for hundreds of projects at
  startup would dominate latency.
- **No network ever.** The picker is fully offline.
- **Stop respecting `~` in paths at score time.** The user's
  home prefix appears in every project's path; matching on it is
  worthless. `internal/tui/rank.go::stripHome` removes it before
  scoring. Don't undo this.
- **The shell shim's stdout-capture is load-bearing.** Only the
  picker (no args / `pick`) needs stdout; every other subcommand
  must print directly. The case in `op.bash` whitelists `""` and
  `pick`; everything else passes through.

## Search ranking

Tier-based, with a recency bonus on top. Higher = better. See
`internal/tui/rank.go` for the constants and tests.

```
tierNameExact   1500
tierNamePrefix  1200
tierNameSubstr  1000
tierNameSplit2   800   "frontendplatform" ↔ name with both halves
tierBranch       500
tierPathSubstr   350
tierPathSplit2   250
tierFuzzy        100   classic fzf chars-in-order fallback
recency bonus  0–200
```

Multi-token AND: tokens split on whitespace, every token must
match. Score per row = sum of per-token tier scores + recency
bonus. Each token's tier is the highest one it qualifies for.

## TUI conventions

- Two surfaces share the bubbletea/lipgloss setup: the picker
  (`internal/tui`) and the first-run form (`internal/firstrun`).
  Both open `/dev/tty` for I/O, force TrueColor, set a dark
  background assumption, and re-init their styles before
  constructing their models. The shell shim captures stdout so
  termenv autodetects "no tty" and degrades to ASCII unless we
  override.
- Cursor is the bubbles fake cursor (always block-shaped, only
  the colour can change). Don't try to use the real terminal
  cursor + DECSCUSR — bubbletea v1.3's renderer overrides any
  cursor positioning we write into the View, so the cursor lands
  at the wrong place. Mode is signalled by colour (cyan in
  insert, amber in normal) plus the prompt colour and the footer
  NORMAL indicator.
- `gg`/`G` in normal mode navigate the list, not the input
  cursor. In a fuzzy picker the list is the primary surface; use
  `0`/`$` for input motions.

## Tests

- Every package has table-driven tests using `t.TempDir()`.
- `internal/tui` has focused tests for: search ranking
  (kit-str scenario, frontendplatform scenario, home-prefix
  stripping, multi-token AND, recency tie-break); vim commands
  (`cc`, `cw`, `dw`, `D`, `r`, `s`, `gg`/`G`); ESC behaviour
  with vim mode on vs off.
- `internal/scanner` covers: nested container repos
  (prezly/new-style layout), worktrees nested under main repos
  (.claude/worktrees layout), prune list still wins inside a
  found repo, OnFound callback fires once per found project.
- `internal/firstrun` covers parsing of the input field
  (blank/whitespace/comma-only/single/multi).

## Things to avoid

- Don't add new config keys without updating
  `internal/config/SetField`, `GetField`, and `Keys()`. The
  `op config` UI dispatches off these — they're the source of
  truth for what the CLI exposes.
- Don't break the no-prompt path for non-interactive
  invocations. `firstrun.Run` falls back to `Defaults()` when
  `/dev/tty` can't be opened so CI / piped invocations don't
  block.
- Don't add a network call. Even an "optional" telemetry pingback
  changes the contract. Out of scope.
