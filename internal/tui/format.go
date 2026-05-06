package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// humanAgo renders a "<n> ago" relative-time string. Zero times
// (no HEAD mtime) come back empty so callers can drop the field.
func humanAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// prettyPath collapses $HOME → ~ and trims overly long paths from the
// left so the basename stays visible.
func prettyPath(p string, w int) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		p = "~" + p[len(home):]
	}
	if w <= 0 || len(p) <= w {
		return p
	}
	base := filepath.Base(p)
	if len(base)+1 >= w {
		return base
	}
	tail := p[len(p)-w+1:]
	return "…" + tail
}

// keyHelp renders pairs of (key, label) into a "ctrl+r rescan ·
// enter pick · …" footer line, with the keys themselves brightened
// so they're scannable.
func keyHelp(pairs ...string) string {
	var out strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		if i > 0 {
			out.WriteString(dimStyle.Render(" · "))
		}
		out.WriteString(keyStyle.Render(pairs[i]))
		out.WriteString(" ")
		out.WriteString(dimStyle.Render(pairs[i+1]))
	}
	return out.String()
}
