// Package firstrun shows a one-time "where do you keep your projects?"
// bubbletea form when op has no config yet. The result is returned to
// the caller (main) which writes it to disk before launching the
// picker. Visual style mirrors the picker so the two surfaces feel
// like one app.
package firstrun

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/inf1nite-lo0p/op/internal/config"
)

// ErrCancelled is returned when the user pressed Ctrl+C in the prompt.
// main treats this as "abort the whole launch", same as cancelling
// the picker.
var ErrCancelled = errors.New("firstrun: cancelled")

// Run shows the welcome form and returns the user's chosen config.
//
// Empty input → defaults (single root = "~"). Comma-separated input →
// roots = each non-empty trimmed segment. The prune list comes from
// config.Defaults() either way, since pruning is universal regardless
// of which roots the user chose.
//
// If no controlling terminal is available (CI, piped stdin, etc.) we
// silently return Defaults() — non-interactive runs shouldn't block.
func Run(ctx context.Context) (config.Config, error) {
	tty, closeTTY, err := openTTY()
	if err != nil {
		// No tty — proceed with defaults rather than blocking.
		return config.Defaults(), nil
	}
	defer closeTTY()

	r := lipgloss.NewRenderer(tty, termenv.WithUnsafe())
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	lipgloss.SetDefaultRenderer(r)
	initStyles()

	m := newModel()

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(tty),
		tea.WithOutput(tty),
	)

	final, err := p.Run()
	if err != nil {
		return config.Config{}, err
	}
	fm := final.(model)
	if fm.cancelled {
		return config.Config{}, ErrCancelled
	}

	return cfgFromInput(fm.input.Value()), nil
}

// cfgFromInput parses the textinput value into a Config. Exposed via
// a free function so tests can lock in the parsing rules without
// driving a full bubbletea program.
func cfgFromInput(line string) config.Config {
	cfg := config.Defaults()
	line = strings.TrimSpace(line)
	if line == "" {
		return cfg
	}
	roots := make([]string, 0, 4)
	for _, seg := range strings.Split(line, ",") {
		seg = strings.TrimSpace(seg)
		if seg != "" {
			roots = append(roots, seg)
		}
	}
	if len(roots) == 0 {
		// User typed only commas/whitespace — fall back to default.
		return cfg
	}
	cfg.Roots = roots
	return cfg
}

// openTTY mirrors the picker's helper so the first-run form can also
// render against the real terminal regardless of stdout capture.
func openTTY() (*os.File, func(), error) {
	if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return f, func() { _ = f.Close() }, nil
	}
	return nil, func() {}, errors.New("no tty")
}

// ----- bubbletea model -----

type model struct {
	input         textinput.Model
	width, height int

	cancelled bool
	submitted bool
}

func newModel() model {
	ti := textinput.New()
	ti.Placeholder = "~/code, /mnt/d/projects, …  or blank for $HOME"
	ti.Prompt = "❯ "
	ti.PromptStyle = promptStyle
	ti.TextStyle = textStyle
	ti.PlaceholderStyle = dimStyle
	ti.Cursor.Style = cursorStyle
	ti.Focus()
	ti.CharLimit = 1024
	ti.Width = 70
	return model{input: ti}
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Keep a slim right gutter and account for app padding.
		w := msg.Width - appStyle.GetHorizontalFrameSize() - 4
		if w < 20 {
			w = 20
		}
		if w > 100 {
			w = 100
		}
		m.input.Width = w
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.submitted = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(" Welcome to op "))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(headingStyle.Render("Where do you keep your git projects?"))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(dimStyle.Render("Comma-separated paths. "))
	b.WriteString(codeStyle.Render("~"))
	b.WriteString(dimStyle.Render(" expands to "))
	b.WriteString(codeStyle.Render("$HOME"))
	b.WriteString(dimStyle.Render("; absolute paths work too — other drives "))
	b.WriteString(codeStyle.Render("/mnt/d"))
	b.WriteString(dimStyle.Render(", "))
	b.WriteString(codeStyle.Render("/Volumes/X"))
	b.WriteString(dimStyle.Render(", network mounts, etc."))
	b.WriteString("\n  ")
	b.WriteString(dimStyle.Render("Leave blank to scan "))
	b.WriteString(codeStyle.Render("$HOME"))
	b.WriteString(dimStyle.Render(" only."))
	b.WriteString("\n\n  ")
	b.WriteString(m.input.View())
	b.WriteString("\n\n  ")

	keys := keyHelp(
		"enter", "save",
		"ctrl+c", "skip",
	)
	b.WriteString(keys)

	return appStyle.Render(b.String())
}

// keyHelp renders pairs of (key, label) into "enter save · ctrl+c skip".
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

// ----- styles -----
//
// All vars, populated in initStyles() after the renderer is rebound
// to /dev/tty in Run. Same pattern as the picker package.

var (
	appStyle     lipgloss.Style
	titleStyle   lipgloss.Style
	headingStyle lipgloss.Style
	dimStyle     lipgloss.Style
	codeStyle    lipgloss.Style
	promptStyle  lipgloss.Style
	cursorStyle  lipgloss.Style
	textStyle    lipgloss.Style
	keyStyle     lipgloss.Style
)

func init() { initStyles() }

func initStyles() {
	appStyle = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#0094B0")).
		Padding(0, 1)
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	codeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5"))
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5DC9F5"))
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DC9F5"))
	textStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFDF5"))
	keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DDDDDD"))
}
