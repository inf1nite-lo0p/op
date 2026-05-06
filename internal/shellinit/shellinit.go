// Package shellinit embeds the shell shim and exposes it as a string
// so the binary can print it via `op-bin shell-init <shell>`.
//
// This lets end-users install op with just two lines:
//
//	go install github.com/inf1nite-lo0p/op/cmd/op-bin@latest
//	echo 'eval "$(op-bin shell-init bash)"' >> ~/.bashrc
//
// — same pattern used by zoxide, starship, direnv, and friends.
// No clone, no separate file to source from a fixed location.
package shellinit

import (
	_ "embed"
	"fmt"
)

//go:embed op.bash
var bashShim string

// Script returns the shim source for the given shell flavour. The
// shim is POSIX-portable enough that the same text works in both
// bash and zsh; we still gate on the name so that an unsupported
// shell yields a clear error instead of silently emitting bash.
func Script(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return bashShim, nil
	default:
		return "", fmt.Errorf("shellinit: unsupported shell %q (try: bash, zsh)", shell)
	}
}
