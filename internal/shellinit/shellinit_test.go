package shellinit

import (
	"strings"
	"testing"
)

func TestScriptBashEmitsShim(t *testing.T) {
	got, err := Script("bash")
	if err != nil {
		t.Fatalf("Script(bash): %v", err)
	}
	// The embedded shim must define the `op` function and contain the
	// stdout-capture branch — those two together are what makes the
	// `cd` magic work.
	if !strings.Contains(got, "op() {") {
		t.Error("shim missing `op() {` function definition")
	}
	if !strings.Contains(got, `command op-bin "$@"`) {
		t.Error("shim missing op-bin pass-through")
	}
}

func TestScriptZshSameAsBash(t *testing.T) {
	bash, _ := Script("bash")
	zsh, err := Script("zsh")
	if err != nil {
		t.Fatalf("Script(zsh): %v", err)
	}
	if bash != zsh {
		t.Error("bash and zsh shims should be identical (POSIX-portable)")
	}
}

func TestScriptUnsupportedShellErrors(t *testing.T) {
	if _, err := Script("fish"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}
