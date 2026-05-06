// Package config loads and persists the user's op config.
//
// The config file lives at ~/.config/op/config.toml. On first run we
// create it with sensible defaults so the user has something to edit.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config is the on-disk shape of ~/.config/op/config.toml.
type Config struct {
	// Roots are the directories op recursively scans for git projects.
	// Leading ~ is expanded against $HOME.
	Roots []string `toml:"roots"`

	// Prune is a list of directory base names skipped during the walk.
	// Anything matching short-circuits before we descend.
	Prune []string `toml:"prune"`

	// VimMode enables a vim-style modal editor in the search input.
	// When true, ESC enters normal mode (hjkl/w/b/e/dd/cw/etc.) and
	// only Ctrl+C exits the picker. When false (the default), ESC
	// cancels the picker and the search box behaves like a normal
	// text input.
	VimMode bool `toml:"vim_mode"`
}

// Defaults returns the config we write on first run. Aimed at "works
// for most setups out of the box" — non-existent roots are silently
// skipped during the walk, so listing several common directory
// conventions costs nothing for users that only have one of them.
func Defaults() Config {
	return Config{
		Roots: []string{
			"~/code",
			"~/projects",
			"~/src",
			"~/work",
			"~/repos",
		},
		Prune: []string{
			// Package managers / vendored deps.
			"node_modules", "bower_components",
			"vendor",
			"Pods",
			// Language build outputs.
			"target", // Rust, Java/Maven
			"build", "dist", "out",
			"bin", "obj", // .NET, Go binaries
			"coverage",
			// Python.
			".venv", "venv", "__pycache__",
			// JS framework caches / outputs.
			".next", ".nuxt", ".svelte-kit", ".astro",
			".turbo", ".cache", ".parcel-cache",
			// Infra.
			".terraform", "cdk.out",
		},
	}
}

// Path returns the absolute path of the config file. It does not
// require the file (or its parent dir) to exist.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("config: locating home dir: %w", err)
	}
	// XDG_CONFIG_HOME wins if set, otherwise ~/.config.
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "op", "config.toml"), nil
}

// Load reads the config file. If it does not exist, it writes the
// defaults to disk and returns them — so the user always has a file
// they can open and edit.
func Load() (Config, error) {
	p, err := Path()
	if err != nil {
		return Config{}, err
	}
	return loadFrom(p)
}

func loadFrom(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		def := Defaults()
		if writeErr := writeTo(path, def); writeErr != nil {
			// Returning defaults even if write failed lets op still run;
			// the user gets a clear error from `op doctor`.
			return def, fmt.Errorf("config: writing defaults to %s: %w", path, writeErr)
		}
		return def, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: reading %s: %w", path, err)
	}

	var c Config
	if err := toml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	return c, nil
}

// Save writes the config file, creating its parent dir if needed.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return writeTo(p, c)
}

func writeTo(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ExpandRoots returns the configured roots with ~ expanded and any
// blank/dup entries removed. Returned paths are absolute when possible.
func (c Config) ExpandRoots() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(c.Roots))
	out := make([]string, 0, len(c.Roots))
	for _, r := range c.Roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		expanded := expandHome(r, home)
		if abs, err := filepath.Abs(expanded); err == nil {
			expanded = abs
		}
		if _, dup := seen[expanded]; dup {
			continue
		}
		seen[expanded] = struct{}{}
		out = append(out, expanded)
	}
	return out, nil
}

// expandHome replaces a leading ~ or ~/ with the user's home dir.
// We only handle the leading-tilde case — full shell-style expansion
// (e.g. ~user) is intentionally out of scope.
func expandHome(p, home string) string {
	switch {
	case p == "~":
		return home
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home, p[2:])
	default:
		return p
	}
}

// SetField updates a config field by its TOML key, parsing the value
// from a string. Used by `op config set <key> <value>`. Returns the
// updated config so the caller can save it.
//
// Keeping the key→field mapping here (rather than in the CLI) lets us
// add new settings without touching main.go — and lets tests cover
// every supported key in one place.
func SetField(c Config, key, value string) (Config, error) {
	switch key {
	case "vim_mode":
		v, err := parseBool(value)
		if err != nil {
			return c, fmt.Errorf("config: %s = %q: %w", key, value, err)
		}
		c.VimMode = v
	default:
		return c, fmt.Errorf("config: unknown key %q (try `op config list`)", key)
	}
	return c, nil
}

// GetField returns a string representation of a config field, for
// `op config get <key>`.
func GetField(c Config, key string) (string, error) {
	switch key {
	case "vim_mode":
		return strconv.FormatBool(c.VimMode), nil
	case "roots":
		return strings.Join(c.Roots, "\n"), nil
	case "prune":
		return strings.Join(c.Prune, "\n"), nil
	}
	return "", fmt.Errorf("config: unknown key %q", key)
}

// Keys returns the list of TOML keys that SetField/GetField recognize,
// in display order.
func Keys() []string {
	return []string{"vim_mode", "roots", "prune"}
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "on", "1":
		return true, nil
	case "false", "no", "off", "0":
		return false, nil
	}
	return false, errors.New("expected true/false (also accepts on/off, yes/no, 1/0)")
}

// AddRoot appends a root to the config and saves it. The path is stored
// as the user typed it (so ~ stays as ~), but we reject obvious garbage.
func AddRoot(c Config, root string) (Config, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return c, errors.New("config: empty root")
	}
	for _, existing := range c.Roots {
		if existing == root {
			return c, nil
		}
	}
	c.Roots = append(c.Roots, root)
	if err := Save(c); err != nil {
		return c, err
	}
	return c, nil
}
