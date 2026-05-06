# op

> A blazing-fast TUI for switching to any of your local git projects (and worktrees) from a fresh shell.

[![CI](https://github.com/inf1nite-lo0p/op/actions/workflows/ci.yml/badge.svg)](https://github.com/inf1nite-lo0p/op/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/inf1nite-lo0p/op.svg)](https://pkg.go.dev/github.com/inf1nite-lo0p/op)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Type `op`, fuzzy-find a project, hit Enter — your shell `cd`s into it.
Sub-10ms cold launch on a cache hit. No network. Optional vim-style modal
editing. Built with [bubbletea](https://github.com/charmbracelet/bubbletea).

---

## Highlights

- **Fast** — cached cold-launch reads JSON and renders. Filesystem walks
  happen in a background goroutine after the picker is already on screen.
- **Worktree-aware** — linked git worktrees are first-class rows, including
  worktrees nested inside the main repo (e.g. `<repo>/.claude/worktrees/`).
- **Recursive container repos** — surfaces independent git repos nested
  inside another repo (e.g. an umbrella workspace whose subfolders are each
  their own repo). Vendored deps are kept out by the prune list.
- **Ergonomic ranking** — tier-based scoring beats letters-in-order fuzzy
  matching for project navigation: name-exact (1500) ≫ name-prefix (1200)
  ≫ name-contains (1000) ≫ split-2 (800) ≫ branch (500) ≫ path (350) ≫
  fuzzy fallback (100). Recency adds up to +200 within a tier so today's
  repo wins ties over last month's.
- **Multi-token AND search** — `kit feat` finds rows that match both
  `kit` *and* `feat`, so you can drill into kit's feature-branch worktrees
  in one query. Two-way splits also work: `frontendplatform` matches
  `technance-platform-frontend` because both halves are substrings.
- **Vim mode (opt-in)** — full vim-style modal editor in the search input
  (`hjkl/w/b/e/gg/G/cw/dw/cc/dd/D/C/s/r<x>/i/a/I/A`). Off by default; flip
  with `op config set vim_mode on`.
- **Offline-only** — never makes a network call. Works on a plane, in a
  Docker container, anywhere.

---

## Demo

```
[ Open Project ]

❯ kit

  ❯  ●  kit                                                         repo · 2h ago
        ~/projects/stridge-foundation/kit

     ↳  str-859-fix-transfer-crypto-processing                  worktree · 1h ago
        ~/projects/stridge-foundation/kit/.claude/worktrees/str-859-…

     ↳  str-851-fix-deposit-flow-bugs                           worktree · 4h ago
        ~/projects/stridge-foundation/kit/.claude/worktrees/str-851-…

     ●  kit-sample                                                  repo · 1d ago
        ~/projects/stridge-foundation/kit-sample

     ●  kit-research                                                repo · 3d ago
        ~/projects/stridge-foundation/kit-research

  17/972 · ↑↓/ctrl+jk select · enter pick · esc normal · ctrl+c exit · ctrl+r rescan
```

---

## Install

### Build from source

Requires Go 1.24+ and [`just`](https://github.com/casey/just).

```sh
git clone git@github.com:inf1nite-lo0p/op.git
cd op
just install
```

This builds `op-bin` into `~/.local/bin/op-bin` and installs the shell
shim to `~/.local/share/op/op.bash`. Override with `INSTALL_DIR` and
`SHELL_DIR` env vars if you want to put them somewhere else.

### Wire up the shell shim

Add this to your `~/.bashrc` or `~/.zshrc`:

```sh
source ~/.local/share/op/op.bash
```

`op` is implemented as a shell function — a child process can't change
the parent shell's working directory, so the binary prints the chosen
absolute path and the shim does the `cd`.

---

## Usage

### CLI

```
op                       # open the picker (default)
op refresh               # force rescan, write cache
op list                  # print all known project paths to stdout
op add <path>            # add a root to ~/.config/op/config.toml
op roots                 # print configured roots
op doctor                # show config + cache health
op config                # show current settings + edit hints
op config get <key>      # print one config value
op config set <key> <v>  # change a config value (e.g. vim_mode on)
op config edit           # open the config file in $EDITOR
```

### Picker keys (insert mode)

| Key                          | Action                          |
| ---------------------------- | ------------------------------- |
| any printable character      | filter (instant)                |
| `↑` `↓` / `Ctrl+P` `Ctrl+N`  | move selection                  |
| `Ctrl+J` `Ctrl+K`            | move selection (vim flavour)    |
| `Ctrl+U` `Ctrl+D`            | page up / page down             |
| `Enter`                      | pick and `cd` into the row      |
| `Esc`                        | cancel — *or* enter normal mode (if vim mode is on) |
| `Ctrl+C`                     | exit picker                     |
| `Ctrl+R`                     | force rescan in the background  |

### Picker keys (vim normal mode, when `vim_mode = true`)

| Group              | Keys                                                |
| ------------------ | --------------------------------------------------- |
| Cursor             | `h l 0 ^ $ w b e gg G`                              |
| Char ops           | `x` (delete), `s` (substitute), `r<x>` (replace)    |
| Line ops           | `dd cc S` (clear), `D` (= d$), `C` (= c$)           |
| Operator + motion  | `dw db de d0 d^ d$`, `cw cb ce c0 c^ c$`            |
| Mode entry         | `i a I A`                                           |
| List nav           | `j k`, arrows, `Ctrl+J/K/N/P`, `Ctrl+U/F` (page)    |
| List jump          | `gg` top, `G` bottom of visible filtered list       |
| Exit               | `Ctrl+C`                                            |

`gg`/`G` navigate the **list** (not the input cursor), since in a fuzzy
picker the list is the primary surface. Use `0`/`$` for input motions.

---

## Configuration

Lives at `~/.config/op/config.toml`. First run writes the defaults:

```toml
roots = ["~/projects", "~/playground"]
prune = ["node_modules", ".next", "vendor", "target", "dist", "build", ".venv", "__pycache__", ".turbo"]
vim_mode = false
```

| Key        | Type           | Default | Description                                               |
| ---------- | -------------- | ------- | --------------------------------------------------------- |
| `roots`    | list of paths  | `~/projects`, `~/playground` | Recursively scanned for git working trees. Leading `~` expands to `$HOME`. |
| `prune`    | list of names  | (see above) | Directory base names to skip during the walk. Exact match — no `**` globs. |
| `vim_mode` | bool           | `false` | When `true`, the search input becomes a vim-style modal editor. `Esc` enters normal mode and `Ctrl+C` is the only exit. |

Quick edits without opening the file:

```sh
op config                       # show current settings + hints
op config set vim_mode on       # toggle a setting
op config edit                  # full editor dive ($VISUAL/$EDITOR/vi)
```

---

## How it works

```
~/.cache/op/projects.json   ← versioned, atomic JSON cache
~/.config/op/config.toml    ← user config

  ┌──────────────────────────────────────────────────┐
  │   op  (shell function)                           │
  │       └─ runs op-bin, captures stdout            │
  │           └─ on a chosen path, `cd` into it      │
  └──────────────────────────────────────────────────┘
                          │
                          ▼
  ┌──────────────────────────────────────────────────┐
  │   op-bin                                         │
  │       1. Read cache → render picker (~10ms)      │
  │       2. Spawn rescan goroutine in background    │
  │       3. Patch visible list when scan finishes   │
  │       4. Print chosen path on stdout             │
  └──────────────────────────────────────────────────┘
```

- **Cache.** Versioned JSON at `$XDG_CACHE_HOME/op/projects.json`
  (defaults to `~/.cache/op/projects.json`). Atomic writes via temp +
  rename so a crash mid-write keeps the previous cache intact. Schema
  mismatch falls back to empty (triggers a full rescan).
- **Discovery.** The scanner walks each root in parallel, classifying
  every directory containing a `.git/` (main repo) or `.git` regular
  file (linked worktree). It descends into main repos so nested
  independent repos surface; `.git/` is always skipped.
- **No `git` binary on the hot path.** The `gitmeta` package parses
  `.git/HEAD` directly. Forking the git process for hundreds of
  projects at startup would dominate latency.
- **No network.** Ever. The only thing that talks to a network-y thing
  is your shell's `cd`.
- **Background rescan.** Every launch fires a goroutine that re-walks
  the filesystem and patches the visible list when results arrive — so
  the cache stays fresh without anyone waiting.

### Why a separate `op-bin` plus shell shim?

A child process cannot change its parent shell's working directory.
The Go binary therefore prints the chosen absolute path on stdout,
and a 13-line bash function (`op.bash`) captures that and does the
`cd`. Same trick `fzf-cd` and friends use. The shim also routes
non-picker subcommands directly to stdout so their output is visible
to the user instead of being captured.

---

## Architecture

```
op/
├── cmd/op-bin/main.go         entry, subcommand dispatch
├── internal/
│   ├── cache/                 versioned JSON, atomic write
│   ├── config/                TOML loader, defaults, set/get fields
│   ├── gitmeta/               parse .git dirs/files (no git subprocess)
│   ├── scanner/               parallel filesystem walker + prune logic
│   └── tui/
│       ├── tui.go             Run, model, Update, View, applyFilter
│       ├── delegate.go        per-row rendering (flex layout)
│       ├── rank.go            tier-based search ranking + recency
│       ├── vim.go             insert/normal-mode handlers + motions
│       ├── styles.go          all lipgloss styles + initStyles
│       ├── format.go          humanAgo, prettyPath, keyHelp
│       └── tty.go             /dev/tty open helper
├── shell/op.bash              the shell shim
├── justfile                   build / test / install
└── .github/workflows/ci.yml   build + test on push
```

Each package is exercised by table-driven tests using `t.TempDir()`
fixtures. `just check` runs `go vet`, `gofmt`, and `go test ./...` —
the same checks CI runs on every push.

---

## Performance notes

The launch path is the central design constraint:

- The cold-cache launch reads `projects.json` and nothing else before
  the picker is interactive. Target: <50ms. Cached `op list` (closest
  proxy) measures ~10ms on the author's machine.
- No filesystem walk, no git subprocess, no network call is allowed in
  the cold path. Anything slow runs in a background goroutine after the
  picker is already on screen.
- First run (no cache) shows a "scanning…" message on stderr and scans
  synchronously. Subsequent launches are always cached.

---

## Development

```sh
just                # list recipes
just build          # → bin/op-bin
just test           # go test ./...
just check          # vet + gofmt + tests (CI runs the same)
just install        # build + copy to ~/.local/bin and ~/.local/share/op
just uninstall      # remove the installed binary and shim
just clean          # rm -rf bin/
```

Tests cover every package. The `tui` package has focused tests for
search ranking (multi-token, recency, home-prefix stripping, two-way
splits) and vim-mode commands (`cc`, `cw`, `dw`, `D`, `r`, `s`,
`gg`/`G`).

---

## Acknowledgements

Built on the [Charm](https://charm.sh/) ecosystem:
[bubbletea](https://github.com/charmbracelet/bubbletea),
[bubbles](https://github.com/charmbracelet/bubbles),
[lipgloss](https://github.com/charmbracelet/lipgloss). TOML parsing by
[go-toml/v2](https://github.com/pelletier/go-toml). Inspired by the
look and feel of Claude Code's `/resume` picker.

## License

MIT — see [LICENSE](LICENSE).
