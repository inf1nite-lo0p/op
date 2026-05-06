package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/inf1nite-lo0p/op/internal/cache"
)

func sampleEntries() []cache.Entry {
	return []cache.Entry{
		{Path: "/p/kit", Name: "kit", Branch: "main", MainRepoPath: "/p/kit", HeadMTime: time.Now().Add(-time.Hour)},
		{Path: "/p/kit-feat", Name: "kit-feat", Branch: "feat-x", IsWorktree: true, MainRepoPath: "/p/kit", HeadMTime: time.Now().Add(-2 * time.Hour)},
		{Path: "/p/api", Name: "api", Branch: "main", MainRepoPath: "/p/api", HeadMTime: time.Now().Add(-3 * time.Hour)},
	}
}

func TestEmptyFilterShowsAllEntries(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	if got, want := len(m.matched), 3; got != want {
		t.Fatalf("matched = %d, want %d", got, want)
	}
	if got := len(m.list.Items()); got != 3 {
		t.Fatalf("list items = %d, want 3", got)
	}
}

func TestFilterNarrowsResults(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})

	// Simulate typing "feat" one rune at a time.
	for _, r := range "feat" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
	}
	if len(m.matched) == 0 {
		t.Fatal("filter should match at least one entry")
	}
	if got := m.entries[m.matched[0]].Path; got != "/p/kit-feat" {
		t.Fatalf("top match = %s, want /p/kit-feat", got)
	}
	if got := m.selectedPath(); got != "/p/kit-feat" {
		t.Fatalf("selected = %s, want /p/kit-feat", got)
	}
}

func TestEnterPicksRow(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.picked != "/p/kit" {
		t.Fatalf("picked = %q, want /p/kit", m.picked)
	}
	if cmd == nil {
		t.Fatal("Enter should return a tea.Quit command")
	}
}

func TestEscCancelsWhenVimModeOff(t *testing.T) {
	// Default config has VimMode=false, in which case ESC must cancel
	// (the conventional "escape out of this picker" behaviour). Vim
	// mode is opt-in via config.
	m := newModel(context.Background(), Options{Initial: sampleEntries()})
	if m.vimMode {
		t.Fatal("vimMode should default to false")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if !m.cancelled {
		t.Fatal("ESC should cancel when vimMode is off")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestEscEntersNormalMode(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	if m.mode != modeInsert {
		t.Fatalf("initial mode = %v, want insert", m.mode)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.cancelled {
		t.Fatal("ESC must not cancel — it should enter normal mode")
	}
	if m.mode != modeNormal {
		t.Fatalf("mode after ESC = %v, want normal", m.mode)
	}
}

func TestCtrlCCancels(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if !m.cancelled {
		t.Fatal("ctrl+c should cancel")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

// typeInto sends each rune of s as a separate KeyMsg and returns
// the updated model.
func typeInto(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
	}
	return m
}

func sendKey(t *testing.T, m model, key string) model {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	return next.(model)
}

func TestVimCommandCC(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "kit")
	m = sendKey(t, m, "esc")
	m = sendKey(t, m, "c")
	m = sendKey(t, m, "c")
	if got := m.input.Value(); got != "" {
		t.Fatalf("after cc, value = %q, want empty", got)
	}
	if m.mode != modeInsert {
		t.Fatalf("after cc, mode = %v, want insert", m.mode)
	}
}

func TestVimCommandDW(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "foo bar baz")
	m = sendKey(t, m, "esc")
	m.input.CursorStart()
	m = sendKey(t, m, "d")
	m = sendKey(t, m, "w")
	if got := m.input.Value(); got != "bar baz" {
		t.Fatalf("after dw, value = %q, want 'bar baz'", got)
	}
	if m.mode != modeNormal {
		t.Fatal("dw should stay in normal mode")
	}
}

func TestVimCommandCW(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "foo bar")
	m = sendKey(t, m, "esc")
	m.input.CursorStart()
	m = sendKey(t, m, "c")
	m = sendKey(t, m, "w")
	if got := m.input.Value(); got != "bar" {
		t.Fatalf("after cw, value = %q, want 'bar'", got)
	}
	if m.mode != modeInsert {
		t.Fatal("cw should switch to insert")
	}
}

func TestVimCommandDToEnd(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "foobar")
	m = sendKey(t, m, "esc")
	m.input.SetCursor(3)
	m = sendKey(t, m, "D")
	if got := m.input.Value(); got != "foo" {
		t.Fatalf("after D, value = %q, want 'foo'", got)
	}
}

func TestVimCommandReplace(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "kit")
	m = sendKey(t, m, "esc")
	m.input.SetCursor(0)
	m = sendKey(t, m, "r")
	m = sendKey(t, m, "b")
	if got := m.input.Value(); got != "bit" {
		t.Fatalf("after r-b, value = %q, want 'bit'", got)
	}
}

func TestVimCommandSubstituteChar(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m = typeInto(t, m, "kit")
	m = sendKey(t, m, "esc")
	m.input.SetCursor(0)
	m = sendKey(t, m, "s")
	if got := m.input.Value(); got != "it" {
		t.Fatalf("after s, value = %q, want 'it'", got)
	}
	if m.mode != modeInsert {
		t.Fatal("s should switch to insert")
	}
}

func TestVimCommandGgGNavigatesList(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	// Pretend the user got a window size — the list needs one to allow
	// cursor movement past the first item.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(model)

	m = sendKey(t, m, "esc")
	m = sendKey(t, m, "G")
	if got, want := m.list.Index(), len(m.list.Items())-1; got != want {
		t.Fatalf("G list index = %d, want %d (last)", got, want)
	}
	m = sendKey(t, m, "g")
	m = sendKey(t, m, "g")
	if got := m.list.Index(); got != 0 {
		t.Fatalf("gg list index = %d, want 0", got)
	}
}

func TestNormalModeMovementAndInsert(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	// Type "kit"
	for _, r := range "kit" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(model)
	}
	// ESC into normal mode
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.mode != modeNormal {
		t.Fatal("not in normal mode")
	}
	// 0 → cursor at 0
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	m = next.(model)
	if got := m.input.Position(); got != 0 {
		t.Fatalf("after '0', cursor = %d, want 0", got)
	}
	// 'i' → back to insert mode
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = next.(model)
	if m.mode != modeInsert {
		t.Fatalf("'i' should switch to insert, got %v", m.mode)
	}
	// dd resets the input
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = next.(model)
	if got := m.input.Value(); got != "" {
		t.Fatalf("after 'dd', value = %q, want empty", got)
	}
}

func TestRescanDonePatchesEntries(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	fresh := []cache.Entry{
		{Path: "/p/new", Name: "new", Branch: "main", MainRepoPath: "/p/new", HeadMTime: time.Now()},
	}
	next, _ := m.Update(rescanDoneMsg{entries: fresh})
	m = next.(model)

	if len(m.entries) != 1 || m.entries[0].Path != "/p/new" {
		t.Fatalf("entries not patched: %+v", m.entries)
	}
	if len(m.matched) != 1 {
		t.Fatalf("matched not refreshed: %v", m.matched)
	}
	if got := m.selectedPath(); got != "/p/new" {
		t.Fatalf("selected = %s, want /p/new", got)
	}
}

func TestArrowKeysMoveCursor(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})

	// We need a window size for the list to know how to move; pretend
	// we got one from the harness.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(model)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if got := m.list.Index(); got != 1 {
		t.Fatalf("index after down = %d, want 1", got)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(model)
	if got := m.list.Index(); got != 0 {
		t.Fatalf("index after up = %d, want 0", got)
	}
}

func TestRankEntriesPrefersRecent(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		{Path: "/p/old-platform", Name: "old-platform", Branch: "main", HeadMTime: now.Add(-90 * 24 * time.Hour)},
		{Path: "/p/recent-platform", Name: "recent-platform", Branch: "main", HeadMTime: now.Add(-30 * time.Minute)},
	}
	got := rankEntries(entries, "platform")
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if entries[got[0]].Path != "/p/recent-platform" {
		t.Fatalf("expected recent first, got %s", entries[got[0]].Path)
	}
}

func TestRankEntriesPrefersTitleMatchOverPathMatch(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		// "kit" buried in a long path
		{Path: "/home/u/projects/stridge/kit/.claude/worktrees/feature-thing", Name: "feature-thing", Branch: "feature", HeadMTime: now.Add(-time.Hour)},
		// kit in the name itself
		{Path: "/home/u/projects/stridge/kit", Name: "kit", Branch: "main", HeadMTime: now.Add(-time.Hour)},
	}
	got := rankEntries(entries, "kit")
	if len(got) == 0 || entries[got[0]].Name != "kit" {
		t.Fatalf("expected kit first, got %v (top=%s)", got, entries[got[0]].Name)
	}
}

func TestRankEntriesMultiTokenAND(t *testing.T) {
	now := time.Now()
	entries := []cache.Entry{
		{Path: "/p/kit", Name: "kit", Branch: "main", HeadMTime: now.Add(-time.Hour)},
		{Path: "/p/kit/.claude/worktrees/str-851-fix", Name: "str-851-fix", Branch: "feat-x", HeadMTime: now.Add(-30 * time.Minute)},
		{Path: "/p/kit/.claude/worktrees/some-other", Name: "some-other", Branch: "main", HeadMTime: now.Add(-2 * time.Hour)},
		{Path: "/p/api", Name: "api", Branch: "main", HeadMTime: now.Add(-5 * time.Hour)},
	}
	// "kit fix" should match only the worktree whose path *and* name
	// fuzzy-match both tokens.
	got := rankEntries(entries, "kit fix")
	if len(got) != 1 {
		t.Fatalf("got %d results, want exactly 1: %v", len(got), got)
	}
	if entries[got[0]].Name != "str-851-fix" {
		t.Fatalf("expected str-851-fix, got %s", entries[got[0]].Name)
	}
}

func TestRankEntriesKitStrScenario(t *testing.T) {
	// Reproduces the exact regression from the user's screenshot:
	// typing "kit str" must surface the kit project + its worktrees
	// (whose paths contain `/kit/` and whose names start with `str-…`)
	// above any unrelated project that happens to have k/i/t and
	// s/t/r letters scattered in its path.
	now := time.Now()
	entries := []cache.Entry{
		// The decoy that previously won — old, no literal "kit"/"str"
		// substring anywhere, just scattered letters.
		{Path: "/home/u/playground/socket.io-react-hook", Name: "socket.io-react-hook", Branch: "main", HeadMTime: now.Add(-180 * 24 * time.Hour)},

		// Other decoys with "kit" buried that shouldn't beat the real kit.
		{Path: "/home/u/projects/codrops/StackSlider", Name: "StackSlider", Branch: "master", HeadMTime: now.Add(-60 * 24 * time.Hour)},

		// The real kit project: literal `kit` name, "stridge" gives us "str".
		{Path: "/home/u/projects/stridge-foundation/kit", Name: "kit", Branch: "main", HeadMTime: now.Add(-8 * time.Hour)},

		// Worktrees of kit: name starts with str-, path contains kit.
		{Path: "/home/u/projects/stridge-foundation/kit/.claude/worktrees/str-859-fix", Name: "str-859-fix", Branch: "str-859-fix", IsWorktree: true, HeadMTime: now.Add(-30 * time.Minute)},
		{Path: "/home/u/projects/stridge-foundation/kit/.claude/worktrees/str-851-deposit", Name: "str-851-deposit", Branch: "str-851-deposit", IsWorktree: true, HeadMTime: now.Add(-3 * time.Hour)},
	}

	got := rankEntries(entries, "kit str")
	if len(got) == 0 {
		t.Fatal("expected at least one match for 'kit str'")
	}

	top := entries[got[0]]
	if top.Name != "kit" {
		t.Fatalf("top result = %q, want 'kit'", top.Name)
	}

	// The top three should all be kit-related (kit project + 2 worktrees);
	// the unrelated decoys must rank below.
	if len(got) < 3 {
		t.Fatalf("expected ≥3 results, got %d", len(got))
	}
	for i := 0; i < 3; i++ {
		e := entries[got[i]]
		if !strings.Contains(strings.ToLower(e.Path), "kit") {
			t.Errorf("result #%d = %q (%s) — expected kit-related", i, e.Name, e.Path)
		}
	}
}

func TestRankEntriesFrontendPlatformScenario(t *testing.T) {
	// User typed "frontendplatform" (no space). The intended target's
	// name is "technance-platform-frontend" — same words, opposite
	// order — so a verbatim substring lookup fails, but a 2-way
	// split ("frontend" + "platform") succeeds. A long worktree path
	// that happens to fuzzy-match all 16 letters in order must NOT
	// outrank the real project.
	now := time.Now()
	entries := []cache.Entry{
		{
			Path:       "/home/u/projects/.../str-859-fix-transfer-crypto-processing-0-amount-and-replace-dialog",
			Name:       "str-859-fix-transfer-crypto-processing-0-amount-and-replace-dialog",
			IsWorktree: true, HeadMTime: now.Add(-time.Hour),
		},
		{
			Path:   "/home/u/projects/technance-foundation/technance-platform-frontend",
			Name:   "technance-platform-frontend",
			Branch: "main", HeadMTime: now.Add(-24 * time.Hour),
		},
	}
	got := rankEntries(entries, "frontendplatform")
	if len(got) == 0 {
		t.Fatal("expected at least one match")
	}
	if entries[got[0]].Name != "technance-platform-frontend" {
		t.Fatalf("top result = %q, want technance-platform-frontend", entries[got[0]].Name)
	}
}

func TestSplitMatch(t *testing.T) {
	cases := []struct {
		tok, hay string
		want     bool
	}{
		{"frontendplatform", "technance-platform-frontend", true},
		{"kitstr", "stridge-foundation-kit", true},
		{"abc", "anything", false}, // <4 chars, can't split
		{"frontendplatform", "react-frontend", false},
		{"foobar", "completely-unrelated-name", false},
	}
	for _, c := range cases {
		if got := splitMatch(c.tok, c.hay); got != c.want {
			t.Errorf("splitMatch(%q, %q) = %v, want %v", c.tok, c.hay, got, c.want)
		}
	}
}

func TestRankEntriesIgnoresHomePrefix(t *testing.T) {
	// Reproduces the user's complaint: typing the username (which
	// appears in /home/<user>/...) should NOT trivially match every
	// project under the home dir. Only projects that have the token
	// in their home-relative path should rank as path matches.
	t.Setenv("HOME", "/home/alice")
	now := time.Now()
	entries := []cache.Entry{
		// Username is only in the home prefix, not in the namespace.
		{Path: "/home/alice/projects/stridge/kit", Name: "kit", HeadMTime: now.Add(-time.Hour)},
		{Path: "/home/alice/projects/stridge/api", Name: "api", HeadMTime: now.Add(-30 * time.Minute)},
		// Username appears in the actual namespace path segment.
		{Path: "/home/alice/projects/alice/op", Name: "op", HeadMTime: now.Add(-2 * time.Hour)},
		{Path: "/home/alice/projects/alice/blog", Name: "blog", HeadMTime: now.Add(-3 * time.Hour)},
	}
	got := rankEntries(entries, "alice")

	// The /alice/ namespace entries must come out on top, even though
	// they're older than the stridge ones. Without home-stripping they
	// would tie on path-substr and lose to recency.
	if len(got) < 2 {
		t.Fatalf("got %d results, want at least 2", len(got))
	}
	for i := 0; i < 2; i++ {
		if !strings.Contains(entries[got[i]].Path, "/alice/op") &&
			!strings.Contains(entries[got[i]].Path, "/alice/blog") {
			t.Fatalf("rank #%d = %q, expected an /alice/ namespace project",
				i, entries[got[i]].Path)
		}
	}
}

func TestRankEntriesEmptyQueryReturnsAll(t *testing.T) {
	entries := []cache.Entry{
		{Path: "/p/a", Name: "a"},
		{Path: "/p/b", Name: "b"},
	}
	got := rankEntries(entries, "")
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
}

func TestCtrlJKMoveCursor(t *testing.T) {
	m := newModel(context.Background(), Options{Initial: sampleEntries(), VimMode: true})
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = m2.(model)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	m = next.(model)
	if got := m.list.Index(); got != 1 {
		t.Fatalf("index after ctrl+j = %d, want 1", got)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m = next.(model)
	if got := m.list.Index(); got != 0 {
		t.Fatalf("index after ctrl+k = %d, want 0", got)
	}
}
