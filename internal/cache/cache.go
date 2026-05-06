// Package cache stores the most recent scan result on disk so that op's
// cold launch is just "read JSON, render". The file is versioned so we
// can change the on-disk shape later without having to support every
// historical schema.
//
// Writes are atomic: we write to a temp file and rename into place.
// A crash mid-write leaves the previous cache intact.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Version is the on-disk schema version. Bump it when you change the
// shape of Entry or File in a backwards-incompatible way.
const Version = 1

// File is the top-level JSON object written to disk.
type File struct {
	Version  int     `json:"version"`
	Projects []Entry `json:"projects"`
}

// Entry mirrors the shape spec'd in PROMPT.md. It is intentionally
// decoupled from internal/scanner.Project so that on-disk format
// changes don't require touching the scanner.
type Entry struct {
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	IsWorktree   bool      `json:"is_worktree"`
	MainRepoPath string    `json:"main_repo_path"`
	Branch       string    `json:"branch"`
	HeadMTime    time.Time `json:"head_mtime"`
}

// Path returns the absolute cache file location, honoring XDG_CACHE_HOME.
func Path() (string, error) {
	if cache := os.Getenv("XDG_CACHE_HOME"); cache != "" {
		return filepath.Join(cache, "op", "projects.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "op", "projects.json"), nil
}

// Load returns the cached projects, or an empty slice if the file is
// missing or version-mismatched. The boolean reports whether a usable
// cache was found — useful for the TUI to decide between rendering
// the cached rows immediately and showing a "first run" spinner.
func Load() ([]Entry, bool, error) {
	p, err := Path()
	if err != nil {
		return nil, false, err
	}
	return loadFrom(p)
}

func loadFrom(path string) ([]Entry, bool, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("cache: reading %s: %w", path, err)
	}

	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		// Corrupt JSON — pretend it's empty rather than crashing.
		return nil, false, nil
	}
	if f.Version != Version {
		// Version mismatch is not an error — we just rescan.
		return nil, false, nil
	}
	return f.Projects, true, nil
}

// Save writes the cache atomically: temp file in the same directory,
// fsync, rename. Same-directory rename is atomic on POSIX, which is
// what keeps a crashed write from losing the previous cache.
func Save(entries []Entry) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return saveTo(p, entries)
}

func saveTo(path string, entries []Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f := File{Version: Version, Projects: entries}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "projects.json.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// If anything below fails, clean up the tmp file.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Age returns the duration since the cache file was last written, or
// 0 + false if the file doesn't exist.
func Age() (time.Duration, bool, error) {
	p, err := Path()
	if err != nil {
		return 0, false, err
	}
	st, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return time.Since(st.ModTime()), true, nil
}
