// Package tui is the bubbletea picker. The launch path is intentionally
// boring: render whatever rows the caller handed us, kick off a
// background rescan, and let the user filter and pick. Nothing here
// runs git, walks the filesystem, or talks to the network.
//
// File layout in this package:
//
//	tui.go        -> Run, model, Update, View, applyFilter (this file)
//	delegate.go   -> projectItem + opDelegate (per-row rendering)
//	rank.go       -> tier-based search ranking + recency bonus
//	vim.go        -> insert/normal mode handlers + word motions
//	styles.go     -> all lipgloss styles + initStyles
//	format.go     -> humanAgo, prettyPath, keyHelp helpers
//	tty.go        -> /dev/tty open helper (so the shell shim can
//	                 capture stdout without breaking the TUI)
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/inf1nite-lo0p/op/internal/cache"
)

// ErrCancelled is returned by Run when the user pressed Ctrl+C. main
// translates this into a quiet non-zero exit so the shell shim doesn't
// try to cd anywhere.
var ErrCancelled = errors.New("tui: cancelled")

// RescanFn runs a fresh scan and returns the resulting cache entries.
// It must respect ctx — Run cancels it on quit. onFound is called by
// the scanner once per project as it's discovered, on a worker
// goroutine; the callback is what drives the picker's live progress
// counter. It can be nil for callers that don't care about progress.
type RescanFn func(ctx context.Context, onFound func()) ([]cache.Entry, error)

// Options configures Run.
type Options struct {
	Initial []cache.Entry
	Rescan  RescanFn

	// VimMode enables modal editing in the search input. When false
	// (the default for new users), ESC cancels the picker and the
	// input is always a plain text field. When true, ESC switches to
	// normal mode (hjkl/cw/dd/…) and only Ctrl+C exits.
	VimMode bool
}

// rescanDoneMsg is delivered when the background rescan finishes.
type rescanDoneMsg struct {
	entries []cache.Entry
	err     error
}

// Run starts the picker. Returns the absolute path the user chose, or
// ErrCancelled if they backed out.
//
// Critical detail: the shell shim captures op-bin's stdout to learn
// which directory to `cd` into. That has two consequences:
//
//  1. We can't render to stdout — we'd be writing into the shim's
//     captured variable. So we open /dev/tty for input and output
//     (same trick fzf uses).
//  2. lipgloss/termenv autodetects colour support by isatty(stdout).
//     With stdout captured, that returns false and lipgloss degrades
//     to ASCII / no colour — the symptom is a beautifully styled
//     bubbles/list rendering as plain monochrome text. We override
//     by forcing TrueColor on a renderer bound to /dev/tty, which is
//     the *real* output device.
func Run(ctx context.Context, opts Options) (string, error) {
	tty, closeTTY, err := openTTYFile()
	if err != nil {
		return "", err
	}
	defer closeTTY()

	// Bind lipgloss's default renderer to the real terminal, then
	// force TrueColor + a dark-background assumption. Done *before*
	// constructing the model so package-level styles created in
	// initStyles pick up the right renderer at the right time.
	r := lipgloss.NewRenderer(tty, termenv.WithUnsafe())
	r.SetColorProfile(termenv.TrueColor)
	r.SetHasDarkBackground(true)
	lipgloss.SetDefaultRenderer(r)
	initStyles()

	m := newModel(ctx, opts)

	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithContext(ctx),
		tea.WithInput(tty),
		tea.WithOutput(tty),
	)

	final, err := p.Run()
	if err != nil {
		return "", err
	}
	fm := final.(model)
	if fm.cancelled || fm.picked == "" {
		return "", ErrCancelled
	}
	return fm.picked, nil
}

// model is the TUI's bubbletea model. Most behaviour is split into
// sibling files (vim handling, ranking, delegate); this file just
// wires the bubbletea lifecycle.
type model struct {
	ctx     context.Context
	rescan  RescanFn
	entries []cache.Entry
	matched []int

	list    list.Model
	input   textinput.Model
	spinner spinner.Model

	vimMode        bool // is modal editing enabled at all?
	mode           editorMode
	pendingOp      string // for two-stroke commands like "dd"/"cc"/"gg"
	pendingReplace bool   // r<x>: next char replaces the one under cursor

	width, height int
	rescanning    bool
	rescanFound   *atomic.Int64 // live counter the scanner increments
	rescanShown   int           // most-recently displayed value of the above
	rescanErr     error
	notice        string
	noticeAt      time.Time

	picked    string
	cancelled bool
}

func newModel(ctx context.Context, opts Options) model {
	// Textinput modelled on bubbletea/examples/textinput: a styled
	// "❯ " prompt + the editable area inline, no border. Less visual
	// noise, more contrast, and the prompt itself is the focus indicator.
	ti := textinput.New()
	ti.Placeholder = "Search…"
	ti.Prompt = "❯ "
	ti.PromptStyle = insertModePromptStyle
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFDF5"))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777"))
	// We use the bubbles "fake cursor" (a Reverse-styled char). It's
	// always block-shaped — bubbletea v1.3's renderer overrides any
	// cursor positioning we write into the View, so the real-cursor +
	// DECSCUSR approach lands the cursor in the wrong place. Mode is
	// signalled by colour instead: cyan in insert, amber in normal.
	ti.Cursor.Style = insertModeCursorStyle
	ti.Focus()
	ti.CharLimit = 256

	l := list.New(nil, opDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(true)
	l.DisableQuitKeybindings()

	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = noticeStyle

	m := model{
		spinner: sp,
		ctx:     ctx,
		rescan:  opts.Rescan,
		entries: opts.Initial,
		input:   ti,
		list:    l,
		vimMode: opts.VimMode,
	}
	m.applyFilter(true)
	return m
}

func (m model) Init() tea.Cmd {
	return m.startRescan()
}

// progressTickMsg drives the live "scanning… N found" counter. We
// poll the atomic counter on a timer rather than rendering on every
// callback, since a scan can find hundreds of projects per millisecond
// — flushing a render that often would hammer the renderer for no
// visible benefit.
type progressTickMsg struct{}

func progressTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return progressTickMsg{}
	})
}

// beginRescan returns the batch of commands to fire when a rescan
// kicks off: the rescan itself, the spinner animation, and the
// progress poll. Callers should set m.rescanning + reset the counter
// before invoking.
func (m *model) beginRescan() tea.Cmd {
	if m.rescan == nil {
		return nil
	}
	if m.rescanFound == nil {
		m.rescanFound = &atomic.Int64{}
	}
	m.rescanFound.Store(0)
	m.rescanShown = 0

	rescan, ctx := m.rescan, m.ctx
	counter := m.rescanFound
	scanCmd := func() tea.Msg {
		entries, err := rescan(ctx, func() { counter.Add(1) })
		return rescanDoneMsg{entries: entries, err: err}
	}
	return tea.Batch(scanCmd, m.spinner.Tick, progressTick())
}

// startRescan is the no-progress path used by Init() — the very first
// rescan happens before any window-size message has arrived, so we
// fire it without animations and rely on the cached rows being
// already on screen. Any user-triggered rescan (Ctrl+R) goes through
// beginRescan instead.
func (m model) startRescan() tea.Cmd {
	if m.rescan == nil {
		return nil
	}
	rescan, ctx := m.rescan, m.ctx
	return func() tea.Msg {
		entries, err := rescan(ctx, nil)
		return rescanDoneMsg{entries: entries, err: err}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// title + 2 blanks + input + 2 blanks + footer ≈ 7 rows;
		// appStyle adds frame padding on top of that.
		hFrame, vFrame := appStyle.GetFrameSize()
		listH := msg.Height - vFrame - 7
		if listH < 5 {
			listH = 5
		}
		listW := msg.Width - hFrame
		if listW < 20 {
			listW = 20
		}
		m.list.SetSize(listW, listH)
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C always exits. With vim mode enabled, that's the only
		// exit (ESC switches modes). With vim mode disabled, ESC also
		// cancels — see updateInsertMode.
		if msg.String() == "ctrl+c" {
			m.cancelled = true
			return m, tea.Quit
		}
		if m.vimMode && m.mode == modeNormal {
			return m.updateNormalMode(msg)
		}
		return m.updateInsertMode(msg)

	case rescanDoneMsg:
		m.rescanning = false
		m.rescanErr = msg.err
		if msg.err != nil {
			m.notice = "rescan failed: " + msg.err.Error()
			m.noticeAt = time.Now()
			return m, nil
		}
		oldSelected := m.selectedPath()
		m.entries = msg.entries
		m.applyFilter(false)
		m.restoreCursor(oldSelected)
		m.notice = fmt.Sprintf("rescanned (%d projects)", len(m.entries))
		m.noticeAt = time.Now()
		return m, nil

	case progressTickMsg:
		// Stop polling once the scan finishes; the rescanDoneMsg
		// arrived already and re-armed nothing.
		if !m.rescanning {
			return m, nil
		}
		if m.rescanFound != nil {
			m.rescanShown = int(m.rescanFound.Load())
		}
		return m, progressTick()

	case spinner.TickMsg:
		// Spinner ticks itself; just forward and stop animating once
		// the scan is done.
		if !m.rescanning {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	// Anything else (non-key messages) — pass to textinput in case
	// it cares (e.g. cursor blink ticks).
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.applyFilter(true)
	}
	return m, cmd
}

// selectedPath returns the path under the cursor, or "" if no rows.
func (m model) selectedPath() string {
	if item, ok := m.list.SelectedItem().(projectItem); ok {
		return item.e.Path
	}
	return ""
}

// restoreCursor moves the list cursor back to the row whose path
// matches target. Used after a rescan replaces the underlying entries.
func (m *model) restoreCursor(target string) {
	if target == "" {
		return
	}
	for i, it := range m.list.Items() {
		if pi, ok := it.(projectItem); ok && pi.e.Path == target {
			m.list.Select(i)
			return
		}
	}
}

// applyFilter rebuilds the list's items from the current input value.
// resetCursor=true is right for filter changes (jump back to the top
// of the new result set); false is right for rescans where we want
// to preserve the user's position.
func (m *model) applyFilter(resetCursor bool) {
	q := strings.TrimSpace(m.input.Value())
	var indices []int
	if q == "" {
		indices = make([]int, len(m.entries))
		for i := range m.entries {
			indices[i] = i
		}
	} else {
		indices = rankEntries(m.entries, q)
	}
	m.matched = indices

	items := make([]list.Item, len(indices))
	for i, idx := range indices {
		items[i] = projectItem{e: m.entries[idx]}
	}
	m.list.SetItems(items)
	if resetCursor {
		m.list.Select(0)
	}
}

func (m model) View() string {
	var b strings.Builder

	// Width the input fills; -4 to leave a slim right gutter.
	if m.width > 0 {
		m.input.Width = m.width - appStyle.GetHorizontalFrameSize() - 4
		if m.input.Width < 10 {
			m.input.Width = 10
		}
	}

	// Title block + blank + input + blank + (optional empty notice) +
	// list + footer. The double-newline after the input is what gives
	// the search bar visual breathing room above the list.
	b.WriteString(titleStyle.Render(" Open Project "))
	b.WriteString("\n\n")
	b.WriteString(m.input.View())
	b.WriteString("\n\n")

	if len(m.entries) == 0 {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("no projects yet — try `op refresh`"))
		b.WriteString("\n")
	}

	b.WriteString(m.list.View())
	b.WriteString(footerStyle.Render(m.renderFooter()))

	return appStyle.Render(b.String())
}

// renderFooter builds the help line at the bottom. The keys shown are
// mode-aware so the next keystroke's effect is always documented.
func (m model) renderFooter() string {
	var keys string
	switch {
	case m.mode == modeNormal:
		keys = keyHelp(
			"hjkl", "move",
			"w/b/e", "word",
			"gg/G", "list top/end",
			"cw/dw", "change/del",
			"cc/dd", "clear",
			"i/a", "insert",
			"enter", "pick",
		)
	case m.vimMode:
		keys = keyHelp(
			"↑↓/ctrl+jk", "select",
			"enter", "pick",
			"esc", "normal",
			"ctrl+c", "exit",
			"ctrl+r", "rescan",
		)
	default:
		keys = keyHelp(
			"↑↓/ctrl+jk", "select",
			"enter", "pick",
			"esc", "cancel",
			"ctrl+r", "rescan",
		)
	}

	// Stats label: plain English when nothing's filtered ("994 projects"),
	// "X of Y" when a filter is active. The old "143/994" format read
	// like a fraction and made people wonder what the slash meant.
	var stats string
	switch {
	case len(m.entries) == 0:
		stats = "no projects"
	case len(m.matched) == len(m.entries):
		stats = fmt.Sprintf("%d projects", len(m.entries))
	default:
		stats = fmt.Sprintf("%d of %d", len(m.matched), len(m.entries))
	}
	out := dimStyle.Render(stats) + dimStyle.Render(" · ")
	if m.mode == modeNormal {
		out += normalModePromptStyle.Render("NORMAL") + dimStyle.Render(" · ")
	}
	out += keys
	if m.rescanning {
		// Spinner + live "found N" counter so the user sees actual
		// progress, not just a static "scanning…" label.
		out += dimStyle.Render(" · ")
		out += m.spinner.View()
		out += " "
		out += noticeStyle.Render("scanning")
		if m.rescanShown > 0 {
			out += dimStyle.Render(fmt.Sprintf(" · %d found", m.rescanShown))
		}
	}
	if m.notice != "" && time.Since(m.noticeAt) < 5*time.Second {
		sep := dimStyle.Render(" · ")
		if m.rescanErr != nil {
			out += sep + errStyle.Render(m.notice)
		} else {
			out += sep + noticeStyle.Render(m.notice)
		}
	}
	return out
}
