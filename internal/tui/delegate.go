package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/inf1nite-lo0p/op/internal/cache"
)

// projectItem adapts cache.Entry to bubbles/list's Item interface.
// FilterValue is what list's own filter would consult — we don't use
// that path (we drive filtering from our own textinput), but the
// interface still requires it.
type projectItem struct {
	e cache.Entry
}

func (p projectItem) Title() string {
	if p.e.IsWorktree {
		return wtArrowStyle.Render("↳ ") + p.e.Name
	}
	return p.e.Name
}

func (p projectItem) Description() string {
	var parts []string
	if p.e.Branch != "" {
		parts = append(parts, branchStyle.Render(p.e.Branch))
	}
	if age := humanAgo(p.e.HeadMTime); age != "" {
		parts = append(parts, age)
	}
	parts = append(parts, prettyPath(p.e.Path, 200))
	return strings.Join(parts, "  ·  ")
}

func (p projectItem) FilterValue() string {
	return p.e.Name + " " + p.e.Branch + " " + p.e.Path
}

// opDelegate renders each project as a two-line block:
//
//	Line 1:  ❯  ●  name                                  repo · 7h ago
//	Line 2:        branch · ~/projects/.../path
//
// The right column on line 1 is right-aligned (flex layout) so the
// eye can scan a single column down the right edge for "what did I
// touch recently". Line 2 is dim metadata; the branch is omitted
// when it would duplicate the row's name (worktrees) or be a noisy
// default ("main"/"master").
//
// Colours follow Claude /resume: only the *active* row's name is
// bright white + cyan indicator; the rest is muted grey, so scanning
// the list stays easy on the eye.
type opDelegate struct{}

func (d opDelegate) Height() int                             { return 2 }
func (d opDelegate) Spacing() int                            { return 1 }
func (d opDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d opDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	p, ok := item.(projectItem)
	if !ok {
		return
	}
	e := p.e
	selected := index == m.Index()

	width := m.Width()
	if width <= 0 {
		width = 100
	}

	// Gutter: indicator (2 chars) + type icon (2 chars) = 4 chars
	// before the name.
	var indicator string
	if selected {
		indicator = activeIndicatorStyle.Render("❯ ")
	} else {
		indicator = "  "
	}

	var typeIcon string
	switch {
	case e.IsWorktree && selected:
		typeIcon = activeTypeIconStyle.Render("↳ ")
	case e.IsWorktree:
		typeIcon = mutedTypeIconStyle.Render("↳ ")
	case selected:
		typeIcon = activeTypeIconStyle.Render("● ")
	default:
		typeIcon = mutedTypeIconStyle.Render("● ")
	}

	var styledName string
	if selected {
		styledName = activeTitleStyle.Render(e.Name)
	} else {
		styledName = mutedTitleStyle.Render(e.Name)
	}

	typeLabel := "repo"
	if e.IsWorktree {
		typeLabel = "worktree"
	}
	right := typeLabel + " · " + humanAgo(e.HeadMTime)
	var styledRight string
	if selected {
		styledRight = activeMetaStyle.Render(right)
	} else {
		styledRight = mutedStyle.Render(right)
	}

	indW := lipgloss.Width(indicator)
	typeW := lipgloss.Width(typeIcon)
	nameW := lipgloss.Width(styledName)
	rightW := lipgloss.Width(styledRight)

	pad := width - indW - typeW - nameW - rightW
	if pad < 1 {
		pad = 1
	}
	line1 := indicator + typeIcon + styledName + strings.Repeat(" ", pad) + styledRight

	// Line 2: dim metadata. Hide branch when uninformative.
	var metaParts []string
	if e.Branch != "" && e.Branch != e.Name && e.Branch != "main" && e.Branch != "master" {
		metaParts = append(metaParts, e.Branch)
	}
	pathW := width - indW - typeW - 2
	if pathW < 20 {
		pathW = 20
	}
	metaParts = append(metaParts, prettyPath(e.Path, pathW))
	meta := mutedStyle.Render(strings.Join(metaParts, "  ·  "))
	// Indent to align with line 1's name column.
	line2 := strings.Repeat(" ", indW+typeW) + meta

	fmt.Fprint(w, line1+"\n"+line2)
}
