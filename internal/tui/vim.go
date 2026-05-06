package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// editorMode is the textinput's current vim-style mode.
type editorMode int

const (
	modeInsert editorMode = iota
	modeNormal
)

// updateInsertMode handles keystrokes when the textinput is the
// active surface — typing fills the search, navigation moves the
// list, ESC switches to normal mode.
func (m model) updateInsertMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Without vim mode, ESC is the user's "I'm done, cancel"
		// signal. With vim mode, it's the gateway into normal mode.
		if !m.vimMode {
			m.cancelled = true
			return m, tea.Quit
		}
		m.mode = modeNormal
		m.pendingOp = ""
		m.input.PromptStyle = normalModePromptStyle
		m.input.Cursor.Style = normalModeCursorStyle
		// Clamp cursor onto a real character (vim convention).
		if pos := m.input.Position(); pos > 0 && pos == len(m.input.Value()) {
			m.input.SetCursor(pos - 1)
		}
		return m, nil
	case "enter":
		if item, ok := m.list.SelectedItem().(projectItem); ok {
			m.picked = item.e.Path
		}
		return m, tea.Quit
	case "up", "ctrl+p", "ctrl+k":
		m.list.CursorUp()
		return m, nil
	case "down", "ctrl+n", "ctrl+j":
		m.list.CursorDown()
		return m, nil
	case "pgup", "ctrl+u":
		m.list.Paginator.PrevPage()
		return m, nil
	case "pgdown", "ctrl+d":
		m.list.Paginator.NextPage()
		return m, nil
	case "ctrl+r":
		if !m.rescanning {
			m.rescanning = true
			return m, m.beginRescan()
		}
		return m, nil
	}

	// Default: textinput handles it (typing, backspace, ctrl+a/e, etc.).
	prev := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prev {
		m.applyFilter(true)
	}
	return m, cmd
}

// updateNormalMode handles a vim subset for the single-line search
// box. Operators (d, c) compose with motions (h/l/w/b/e/0/$/^);
// the doubled forms (dd, cc) operate on the whole line; D and C are
// shortcuts for d$ and c$. We also support s (substitute char), S
// (substitute line), r<x> (replace char), and i/a/I/A for re-entry.
//
// gg/G are remapped to navigate the *list* (not the input cursor) —
// in a fuzzy picker the list is the primary surface, so "go to
// first/last visible match" matches the user's intuition. Use 0/$
// for input-position motions instead.
//
// j/k always navigate the list — even mid-operator-pending — so the
// user can browse the result set without exiting normal mode.
func (m model) updateNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Pending replace: r<x> swaps the char under the cursor with x.
	if m.pendingReplace {
		m.pendingReplace = false
		if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
			val := m.input.Value()
			pos := m.input.Position()
			if pos < len(val) {
				r := string(msg.Runes[0])
				m.input.SetValue(val[:pos] + r + val[pos+1:])
				m.input.SetCursor(pos)
				m.applyFilter(true)
			}
		}
		return m, nil
	}

	// List navigation works in both modes and at any time.
	switch key {
	case "j", "down", "ctrl+j", "ctrl+n":
		m.list.CursorDown()
		return m, nil
	case "k", "up", "ctrl+k", "ctrl+p":
		m.list.CursorUp()
		return m, nil
	case "enter":
		if item, ok := m.list.SelectedItem().(projectItem); ok {
			m.picked = item.e.Path
		}
		return m, tea.Quit
	case "ctrl+r":
		if !m.rescanning {
			m.rescanning = true
			return m, m.beginRescan()
		}
		return m, nil
	case "ctrl+u":
		m.list.Paginator.PrevPage()
		return m, nil
	case "ctrl+f":
		m.list.Paginator.NextPage()
		return m, nil
	case "esc":
		m.pendingOp = ""
		return m, nil
	}

	// Two-stroke "dd" / "cc" — operate on the entire line.
	if m.pendingOp == "d" && key == "d" {
		m.pendingOp = ""
		m.input.SetValue("")
		m.input.CursorStart()
		m.applyFilter(true)
		return m, nil
	}
	if m.pendingOp == "c" && key == "c" {
		m.pendingOp = ""
		m.input.SetValue("")
		m.input.CursorStart()
		m.enterInsertMode()
		m.applyFilter(true)
		return m, nil
	}

	// Doubled "gg" — jump to the top of the visible filtered list.
	if m.pendingOp == "g" && key == "g" {
		m.pendingOp = ""
		if len(m.list.Items()) > 0 {
			m.list.Select(0)
		}
		return m, nil
	}

	val := m.input.Value()
	pos := m.input.Position()

	// Motion table. Each entry computes the *target* cursor position
	// for a single keypress; operators treat (pos, target) as a range.
	if target, ok := motionTarget(key, val, pos); ok {
		switch m.pendingOp {
		case "":
			if target < 0 {
				target = 0
			}
			if target > len(val) {
				target = len(val)
			}
			// 'l'/'w'/'e' shouldn't park cursor on the trailing nothing.
			if (key == "l" || key == "w" || key == "e") && target == len(val) && len(val) > 0 {
				target = len(val) - 1
			}
			m.input.SetCursor(target)
		case "d":
			m.deleteRange(pos, target)
		case "c":
			m.deleteRange(pos, target)
			m.enterInsertMode()
		}
		m.pendingOp = ""
		return m, nil
	}

	// Operators and standalone commands.
	switch key {
	case "d":
		m.pendingOp = "d"
	case "c":
		m.pendingOp = "c"
	case "g":
		m.pendingOp = "g" // wait for second g (-> jump list to top)
	case "G":
		// Jump to the *bottom* of the visible filtered list.
		if n := len(m.list.Items()); n > 0 {
			m.list.Select(n - 1)
		}
	case "D":
		m.deleteRange(pos, len(val))
	case "C":
		m.deleteRange(pos, len(val))
		m.enterInsertMode()
	case "x":
		m.deleteRange(pos, pos+1)
	case "s":
		// Substitute char: delete the char and drop into insert.
		if pos < len(val) {
			m.deleteRange(pos, pos+1)
			m.enterInsertMode()
		}
	case "S":
		// Same as cc.
		m.input.SetValue("")
		m.input.CursorStart()
		m.enterInsertMode()
		m.applyFilter(true)
	case "r":
		m.pendingReplace = true
	case "i":
		m.enterInsertMode()
	case "a":
		if pos < len(val) {
			m.input.SetCursor(pos + 1)
		}
		m.enterInsertMode()
	case "I":
		m.input.CursorStart()
		m.enterInsertMode()
	case "A":
		m.input.CursorEnd()
		m.enterInsertMode()
	}
	return m, nil
}

// deleteRange removes [start, end) from the input, filters, and parks
// the cursor at start (clamped to the new length, vim-style).
func (m *model) deleteRange(start, end int) {
	val := m.input.Value()
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(val) {
		end = len(val)
	}
	if start == end {
		return
	}
	m.input.SetValue(val[:start] + val[end:])
	newVal := m.input.Value()
	cursor := start
	if cursor >= len(newVal) && len(newVal) > 0 {
		cursor = len(newVal) - 1
	}
	if cursor < 0 {
		cursor = 0
	}
	m.input.SetCursor(cursor)
	m.applyFilter(true)
}

// motionTarget returns (targetPos, true) when key is a recognised
// motion. Any range-aware command (d/c) feeds its result back as a
// deletion span; bare keypresses just move the cursor.
func motionTarget(key, s string, pos int) (int, bool) {
	switch key {
	case "h":
		if pos > 0 {
			return pos - 1, true
		}
		return pos, true
	case "l":
		if pos < len(s) {
			return pos + 1, true
		}
		return pos, true
	case "0", "^":
		return 0, true
	case "$":
		return len(s), true
	case "w":
		return nextWordStart(s, pos), true
	case "b":
		return prevWordStart(s, pos), true
	case "e":
		// 'e' lands ON the end-of-word char. For operators, that
		// should INCLUDE that char, so we return one past it.
		return nextWordEnd(s, pos) + 1, true
	}
	return 0, false
}

func (m *model) enterInsertMode() {
	m.mode = modeInsert
	m.pendingOp = ""
	m.pendingReplace = false
	m.input.PromptStyle = insertModePromptStyle
	m.input.Cursor.Style = insertModeCursorStyle
}

// isWordChar matches vim's default \w word definition.
func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_'
}

// nextWordStart returns the position of the start of the next word
// after pos (vim 'w'). Stops at end-of-string.
func nextWordStart(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	startInWord := isWordChar(s[pos])
	for pos < len(s) && isWordChar(s[pos]) == startInWord {
		pos++
	}
	for pos < len(s) && s[pos] == ' ' {
		pos++
	}
	return pos
}

// prevWordStart returns the position of the start of the previous
// word before pos (vim 'b'). Stops at 0.
func prevWordStart(s string, pos int) int {
	if pos == 0 {
		return 0
	}
	pos--
	for pos > 0 && s[pos] == ' ' {
		pos--
	}
	if pos == 0 {
		return 0
	}
	endInWord := isWordChar(s[pos])
	for pos > 0 && isWordChar(s[pos-1]) == endInWord {
		pos--
	}
	return pos
}

// nextWordEnd returns the position of the *end* of the next word
// after pos (vim 'e').
func nextWordEnd(s string, pos int) int {
	if pos >= len(s)-1 {
		return len(s) - 1
	}
	pos++
	for pos < len(s) && !isWordChar(s[pos]) {
		pos++
	}
	for pos < len(s)-1 && isWordChar(s[pos+1]) {
		pos++
	}
	return pos
}
