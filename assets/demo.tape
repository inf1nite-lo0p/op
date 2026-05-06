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
Type 'export XDG_CONFIG_HOME="$PWD/.demo/config"'
Enter
Type 'export XDG_CACHE_HOME="$PWD/.demo/cache"'
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

# ----- act 5: pick ------------------------------------------------

Enter
Sleep 600ms

# ----- act 6: show that we cd'd ----------------------------------

Type "pwd"
Enter
Sleep 1.8s

# fade
Sleep 800ms
