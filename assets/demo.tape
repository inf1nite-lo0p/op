# op demo — render with `just demo` (or `vhs assets/demo.tape`).
#
# Requires:
#   - vhs       https://github.com/charmbracelet/vhs
#   - ttyd      https://github.com/tsl0922/ttyd
#   - ffmpeg    https://ffmpeg.org/
#
# Tape syntax: https://github.com/charmbracelet/vhs#vhs-command-reference

Output assets/demo.gif

# ----- frame -------------------------------------------------------

Set Shell      "bash"
Set FontSize   16
Set Width      1200
Set Height     720
Set Theme      "Catppuccin Mocha"
Set TypingSpeed 70ms
Set Padding    20

# ----- setup (hidden) ---------------------------------------------
#
# We `Hide` the setup commands so they don't show up in the GIF —
# the user just sees the picker doing its thing. The setup:
#   1. Scopes op's config + cache to the seeded demo dir, so we
#      don't show real project paths or pollute the user's config.
#   2. Defines the `op` shell function from the embedded shim, the
#      same way the README's install instructions wire it up.
#
# `just demo` runs assets/seed-demo.sh first, which creates the
# .demo/ tree and pre-warms the cache.

Hide
# Pin the absolute path of the demo dir BEFORE we cd anywhere — the
# rc file lives under it and we don't want $PWD-relative lookups to
# break once we cd into the fake $HOME.
Type 'OP_DEMO="$PWD/.demo"'
Enter
# Source the prompt rc the seed script wrote for us. Two effects:
#   - turns off bash's readline mode-in-prompt (`@` indicator)
#   - sets PROMPT_COMMAND so PS1 always shows the home-relative cwd,
#     so post-pick the prompt itself proves the `cd` worked.
Type 'source "$OP_DEMO/promptrc"'
Enter
# Scope op to the seeded fake home + xdg dirs so paths render as
# `~/code/acme/...` and the recorder's real config / cache stay
# untouched. seed-demo.sh laid all of these down.
Type 'export HOME="$OP_DEMO/home"'
Enter
Type 'export XDG_CONFIG_HOME="$OP_DEMO/config"'
Enter
Type 'export XDG_CACHE_HOME="$OP_DEMO/cache"'
Enter
Type 'cd "$HOME"'
Enter
Type 'eval "$(op-bin shell-init bash)"'
Enter
Type 'clear'
Enter
Show

# ----- act 1: launch ----------------------------------------------

Type "op"
Sleep 400ms
Enter
Sleep 1.5s

# ----- act 2: type a query ----------------------------------------

Type "api"
Sleep 1.5s

# ----- act 3: refine to feature-branch worktrees ------------------

Type " feat"
Sleep 1.8s

# ----- act 4: navigate the list -----------------------------------

Down
Sleep 350ms
Down
Sleep 600ms

# ----- act 5: pick + show we cd'd ----------------------------------
#
# After Enter the shell `cd`s into the picked worktree. PS1 (set by
# .demo/promptrc) automatically shows the home-relative cwd, so the
# prompt itself is the proof — no need to run pwd.

Enter
Sleep 2.5s
