# shellcheck shell=bash
# op shell shim. Source this from your .bashrc / .zshrc:
#
#   source ~/.local/share/op/op.bash
#
# The TUI binary is `op-bin`. We wrap it in a shell function so the
# picker can `cd` us into the chosen path — a child process can't
# change the parent shell's working directory, so the binary prints
# the path and the function does the cd.

op() {
  # Subcommands that print human-readable output need to bypass the
  # stdout-capture below, otherwise their output gets eaten by the
  # `target=$(…)` substitution. The default (no args) and `pick` open
  # the picker, which DOES need stdout capture so we can `cd` into
  # the chosen path.
  case "${1-}" in
    "" | pick) ;; # fall through to the picker path
    *)
      command op-bin "$@"
      return $?
      ;;
  esac

  local target
  # Capture stdout (the path), let stderr (status, errors) through.
  target="$(command op-bin "$@")" || return $?
  if [ -n "$target" ] && [ -d "$target" ]; then
    cd "$target" || return $?
  fi
}
