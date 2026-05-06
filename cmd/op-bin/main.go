// op-bin is the picker binary behind the `op` shell shim. The shim
// captures stdout (which contains a single chosen path) and `cd`s into
// it. All status, errors, and diagnostics go to stderr so they don't
// pollute the path the shim is about to consume.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/inf1nite-lo0p/op/internal/cache"
	"github.com/inf1nite-lo0p/op/internal/config"
	"github.com/inf1nite-lo0p/op/internal/firstrun"
	"github.com/inf1nite-lo0p/op/internal/scanner"
	"github.com/inf1nite-lo0p/op/internal/shellinit"
	"github.com/inf1nite-lo0p/op/internal/tui"
)

func main() {
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch cmd {
	case "", "pick":
		err = runPick(ctx)
	case "refresh":
		err = runRefresh(ctx)
	case "list":
		err = runList()
	case "roots":
		err = runRoots()
	case "add":
		err = runAdd(args[1:])
	case "doctor":
		err = runDoctor(ctx)
	case "config":
		err = runConfig(args[1:])
	case "shell-init":
		err = runShellInit(args[1:])
	case "-h", "--help", "help":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "op: unknown command %q\n\n", cmd)
		printUsage(os.Stderr)
		os.Exit(2)
	}

	if err != nil {
		// errPickerCancelled is the user pressing Esc/Ctrl+C in the
		// picker. Treat as a clean non-zero exit so the shim doesn't
		// `cd` anywhere — but no scary error to print.
		if errors.Is(err, tui.ErrCancelled) {
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "op: %v\n", err)
		os.Exit(1)
	}
}

func printUsage(w *os.File) {
	fmt.Fprintf(w, `op — fuzzy-pick a git project to cd into

Usage:
  op                 open picker (default)
  op refresh         force rescan, write cache
  op list            print all known project paths
  op add <path>      add a root to ~/.config/op/config.toml
  op roots           print configured roots
  op doctor          show config + cache health
  op config          show current settings + edit hints
  op config get <k>  print one config value
  op config set <k> <v>  change a config value (e.g. vim_mode on)
  op config edit     open the config file in $EDITOR
  op shell-init <shell>  print the shell shim (bash or zsh) — eval into your rc
  op help            show this message
`)
}

// runPick is the default mode: render the cached rows immediately and
// rescan in the background. Anything blocking here costs every launch.
func runPick(ctx context.Context) error {
	if err := ensureConfigured(ctx); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		// Don't fail the picker for a config error — log to stderr,
		// keep going. The picker will scan with empty config (no roots)
		// and politely tell the user.
		fmt.Fprintf(os.Stderr, "op: %v\n", err)
	}

	cached, ok, err := cache.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "op: %v\n", err)
	}

	// First-run path: no cache, nothing to render. Scan synchronously
	// so the picker has rows. Show a live spinner + counter on stderr
	// so the user has feedback for what can be a multi-second scan
	// (especially with $HOME as the only root).
	if !ok {
		entries, scanErr := progressScan(ctx, cfg, "First-time scan in progress…")
		if scanErr != nil {
			return scanErr
		}
		cached = entries
	}

	// Hand control to the TUI. It is responsible for kicking the
	// background rescan itself so cancellation lives next to the UI.
	choice, err := tui.Run(ctx, tui.Options{
		Initial: cached,
		VimMode: cfg.VimMode,
		Rescan: func(ctx context.Context, onFound func()) ([]cache.Entry, error) {
			projects, err := scanFromConfig(ctx, cfg, onFound)
			if err != nil {
				return nil, err
			}
			entries := projectsToEntries(projects)
			if err := cache.Save(entries); err != nil {
				// Saving the cache is best-effort: a rescan that can't
				// persist still has value for the running session.
				fmt.Fprintf(os.Stderr, "op: cache save: %v\n", err)
			}
			return entries, nil
		},
	})
	if err != nil {
		return err
	}
	// Picker prints the chosen path on stdout — and *only* the path,
	// no trailing newline trickery, so the shim sees something clean.
	fmt.Println(choice)
	return nil
}

func runRefresh(ctx context.Context) error {
	if err := ensureConfigured(ctx); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_, err = progressScan(ctx, cfg, "Refreshing your project list…")
	return err
}

func runList() error {
	entries, ok, err := cache.Load()
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "op: no cache — run `op refresh`")
		return nil
	}
	for _, e := range entries {
		fmt.Println(e.Path)
	}
	return nil
}

func runRoots() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, r := range cfg.Roots {
		fmt.Println(r)
	}
	return nil
}

func runAdd(args []string) error {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return errors.New("usage: op add <path>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, err := config.AddRoot(cfg, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "added root: %s\n", args[0])
	return nil
}

func runDoctor(ctx context.Context) error {
	cfgPath, err := config.Path()
	if err != nil {
		return err
	}
	cachePath, err := cache.Path()
	if err != nil {
		return err
	}
	fmt.Printf("config:   %s\n", cfgPath)
	fmt.Printf("cache:    %s\n", cachePath)

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("config status: ERROR — %v\n", err)
	} else {
		vim := "off"
		if cfg.VimMode {
			vim = "on"
		}
		fmt.Printf("config status: ok (%d root(s), %d prune, vim_mode %s)\n",
			len(cfg.Roots), len(cfg.Prune), vim)
	}

	roots, err := cfg.ExpandRoots()
	if err != nil {
		fmt.Printf("roots: ERROR — %v\n", err)
	} else {
		for _, r := range roots {
			st, err := os.Stat(r)
			switch {
			case err != nil:
				fmt.Printf("  ✗ %s (%v)\n", r, err)
			case !st.IsDir():
				fmt.Printf("  ✗ %s (not a directory)\n", r)
			default:
				fmt.Printf("  ✓ %s\n", r)
			}
		}
	}

	if age, ok, err := cache.Age(); err != nil {
		fmt.Printf("cache:   ERROR — %v\n", err)
	} else if !ok {
		fmt.Println("cache:   not yet built (run `op refresh`)")
	} else {
		fmt.Printf("cache:   %s old\n", age.Round(time.Second))
	}

	if entries, ok, err := cache.Load(); err != nil {
		fmt.Printf("entries: ERROR — %v\n", err)
	} else if !ok {
		fmt.Println("entries: 0 (no cache)")
	} else {
		fmt.Printf("entries: %d\n", len(entries))
	}

	_ = ctx
	return nil
}

// runConfig dispatches `op config [...]`. With no args, prints the
// current config + a usage hint — that's the discovery path. With
// args (`get`/`set`/`edit`), it acts on the file directly.
func runConfig(args []string) error {
	if len(args) == 0 {
		return runConfigShow()
	}
	switch args[0] {
	case "get":
		return runConfigGet(args[1:])
	case "set":
		return runConfigSet(args[1:])
	case "edit":
		return runConfigEdit()
	default:
		return fmt.Errorf("unknown subcommand %q (try: get, set, edit, or no args)", args[0])
	}
}

// Shared styles for `op config`. Colours match the picker so the two
// surfaces feel like one app. lipgloss auto-detects colour support
// against stdout — pipe to a file or grep and these silently degrade
// to plain text.
var (
	cfgTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#0094B0")).
			Padding(0, 1)
	cfgLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#5DC9F5"))
	cfgValueOnStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7BC97B"))
	cfgValueOffStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	cfgDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	cfgRuleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#444444"))
	cfgCmdStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#DDDDDD"))
)

func runConfigShow() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	p, err := config.Path()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	prettyP := p
	if home != "" && strings.HasPrefix(p, home) {
		prettyP = "~" + p[len(home):]
	}

	var b strings.Builder

	b.WriteString(cfgTitleStyle.Render(" Settings "))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(cfgLabelStyle.Render("config"))
	b.WriteString("    ")
	b.WriteString(cfgDimStyle.Render(prettyP))
	b.WriteString("\n\n")

	// vim_mode: scalar bool, rendered inline with a coloured value.
	var vimVal string
	if cfg.VimMode {
		vimVal = cfgValueOnStyle.Render("on")
	} else {
		vimVal = cfgValueOffStyle.Render("off")
	}
	b.WriteString("  ")
	b.WriteString(cfgLabelStyle.Render("vim_mode"))
	b.WriteString("  ")
	b.WriteString(vimVal)
	b.WriteString("\n\n")

	// roots: section + indented entries.
	b.WriteString("  ")
	b.WriteString(cfgLabelStyle.Render("roots"))
	b.WriteString("\n")
	if len(cfg.Roots) == 0 {
		b.WriteString("    ")
		b.WriteString(cfgDimStyle.Render("(none — add with `op add <path>`)"))
		b.WriteString("\n")
	} else {
		for _, r := range cfg.Roots {
			b.WriteString("    ")
			b.WriteString(cfgDimStyle.Render(r))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// prune: same section style; values joined inline since they're
	// short tokens.
	b.WriteString("  ")
	b.WriteString(cfgLabelStyle.Render("prune"))
	b.WriteString("\n    ")
	b.WriteString(cfgDimStyle.Render(strings.Join(cfg.Prune, "  ·  ")))
	b.WriteString("\n\n")

	// Footer hints. The shell commands themselves get the bright
	// "key" treatment so the eye picks them out at a glance.
	b.WriteString("  ")
	b.WriteString(cfgRuleStyle.Render(strings.Repeat("─", 40)))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(cfgDimStyle.Render("Toggle a value  "))
	b.WriteString(cfgCmdStyle.Render("op config set <key> <value>"))
	b.WriteString("\n  ")
	b.WriteString(cfgDimStyle.Render("Open in editor  "))
	b.WriteString(cfgCmdStyle.Render("op config edit"))
	b.WriteString("\n")

	fmt.Print(b.String())
	return nil
}

func runConfigGet(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: op config get <key>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	v, err := config.GetField(cfg, args[0])
	if err != nil {
		return err
	}
	fmt.Println(v)
	return nil
}

func runConfigSet(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: op config set <key> <value>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg, err = config.SetField(cfg, args[0], args[1])
	if err != nil {
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "set %s = %s\n", args[0], args[1])
	return nil
}

func runConfigEdit() error {
	p, err := config.Path()
	if err != nil {
		return err
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, p)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runShellInit prints the shell shim for the requested shell. The
// intended usage is `eval "$(op-bin shell-init bash)"` in the user's
// rc file — same pattern as `zoxide init bash`, `starship init bash`.
// This is what makes a `go install` setup work without users having
// to clone the repo for the shell file.
func runShellInit(args []string) error {
	shell := "bash"
	if len(args) > 0 {
		shell = args[0]
	}
	script, err := shellinit.Script(shell)
	if err != nil {
		return err
	}
	// Pass through to stdout: the user has wrapped this call in an
	// `eval "$(...)"` so they want stdout content, not stderr.
	fmt.Print(script)
	return nil
}

// ensureConfigured runs the first-run prompt the very first time op
// is launched (no config file yet) and writes the user's chosen
// config. After this returns successfully, config.Load is guaranteed
// to find a file on disk. Subcommands that don't actually scan
// (config/doctor/list/roots) skip this check and rely on Load's
// auto-write fallback.
func ensureConfigured(ctx context.Context) error {
	p, err := config.Path()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		// Config already exists — nothing to do.
		return nil
	}

	cfg, err := firstrun.Run(ctx)
	if err != nil {
		if errors.Is(err, firstrun.ErrCancelled) {
			// User Ctrl+C'd the prompt — exit quietly without
			// writing anything; same exit semantics as cancelling
			// the picker itself.
			return tui.ErrCancelled
		}
		return err
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	return nil
}

// progressScan performs a scanFromConfig call with a live spinner +
// project counter on stderr so the user sees real-time feedback for
// what can be a multi-second walk. Used by both the first-run cold
// scan inside runPick and `op refresh`. Non-TTY stderr (CI, piped
// invocations) falls back to plain start/finish lines so logs don't
// fill up with escape sequences.
//
// The banner is the headline shown above the spinner (e.g. "First-
// time scan in progress…" or "Refreshing your project list…").
func progressScan(ctx context.Context, cfg config.Config, banner string) ([]cache.Entry, error) {
	const (
		ansiCyan      = "\x1b[36m"
		ansiGreen     = "\x1b[32m"
		ansiBold      = "\x1b[1m"
		ansiDim       = "\x1b[2m"
		ansiReset     = "\x1b[0m"
		ansiClearLine = "\r\x1b[K"
	)

	tty := stderrIsTTY()

	if tty {
		fmt.Fprintf(os.Stderr, "\n%s%s%s%s\n\n", ansiBold, ansiCyan, banner, ansiReset)
	} else {
		fmt.Fprintln(os.Stderr, "op: "+strings.ToLower(banner))
	}

	var found atomic.Int64
	doneCh := make(chan struct{})

	if tty {
		go func() {
			// Braille spinner — same family the picker uses.
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			i := 0
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-doneCh:
					return
				case <-ticker.C:
					n := found.Load()
					fmt.Fprintf(os.Stderr,
						"%s  %s%s%s Scanning your projects… %s%d%s found",
						ansiClearLine, ansiCyan, frames[i%len(frames)], ansiReset,
						ansiBold, n, ansiReset)
					i++
				}
			}
		}()
	}

	t := time.Now()
	projects, scanErr := scanFromConfig(ctx, cfg, func() { found.Add(1) })
	elapsed := time.Since(t).Round(time.Millisecond)
	close(doneCh)
	if tty {
		// Tiny pause to make sure the spinner goroutine has stopped
		// writing before we overwrite its line.
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(os.Stderr, ansiClearLine)
	}

	if scanErr != nil {
		return nil, scanErr
	}

	entries := projectsToEntries(projects)
	if err := cache.Save(entries); err != nil {
		fmt.Fprintf(os.Stderr, "op: cache save: %v\n", err)
	}

	if tty {
		fmt.Fprintf(os.Stderr,
			"  %s✓%s Found %s%d%s projects in %s%s%s.\n\n",
			ansiGreen, ansiReset,
			ansiBold, len(entries), ansiReset,
			ansiDim, elapsed, ansiReset)
	} else {
		fmt.Fprintf(os.Stderr, "op: scanned %d projects in %s\n", len(entries), elapsed)
	}
	return entries, nil
}

// stderrIsTTY reports whether stderr is connected to an interactive
// terminal — used to decide whether to animate progress output. The
// shell shim never captures stderr, so this is a reliable signal.
func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// scanFromConfig is the shared "config → scanner.Options → run scan"
// glue. It belongs in main rather than scanner because it expands
// ~ via config and is the only adapter we need. onFound may be nil
// when the caller doesn't care about progress (e.g. `op refresh`
// from the CLI, where we just print the final count to stderr).
func scanFromConfig(ctx context.Context, cfg config.Config, onFound func()) ([]scanner.Project, error) {
	roots, err := cfg.ExpandRoots()
	if err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, nil
	}
	return scanner.Scan(ctx, scanner.Options{
		Roots:    roots,
		Prune:    scanner.PruneSet(cfg.Prune),
		MaxDepth: 8,
		OnFound:  onFound,
	})
}

func projectsToEntries(ps []scanner.Project) []cache.Entry {
	out := make([]cache.Entry, 0, len(ps))
	for _, p := range ps {
		out = append(out, cache.Entry{
			Path:         p.Path,
			Name:         p.Name,
			IsWorktree:   p.IsWorktree,
			MainRepoPath: p.MainRepoPath,
			Branch:       p.Branch,
			HeadMTime:    time.Unix(0, p.HeadMTime),
		})
	}
	return out
}
