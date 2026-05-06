package tui

import "os"

// openTTYFile returns a single *os.File opened on /dev/tty for both
// input and output, plus a closer. We hand the same file to bubbletea
// (for I/O) and to lipgloss (so its renderer can auto-detect colour
// support against the *real* terminal, not the shim-captured stdout).
//
// Falls back to (stdin, stderr) on platforms or environments where
// /dev/tty isn't openable — bubbletea will still render via stderr,
// which the shim doesn't capture.
func openTTYFile() (*os.File, func(), error) {
	if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return f, func() { _ = f.Close() }, nil
	}
	// Fall through: use stderr (write) duped against stdin (read) is
	// awkward, so just return stderr — bubbletea handles input via
	// the controlling process. The shell shim doesn't capture stderr.
	return os.Stderr, func() {}, nil
}
