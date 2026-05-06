// Package gitmeta reads enough of a git repository's on-disk layout to
// classify it without ever shelling out to the git binary. That's the
// whole point of op being fast: parsing 200 bytes of HEAD beats forking
// git every time.
//
// Two kinds of working trees exist:
//
//   - Main: a directory containing a `.git/` subdirectory.
//   - Worktree: a directory containing a `.git` regular file whose
//     contents are `gitdir: <absolute path to the worktree's gitdir>`.
//     That gitdir lives under the main repo at
//     `<main>/.git/worktrees/<name>/`.
package gitmeta

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Kind tells main repos apart from linked worktrees.
type Kind int

const (
	KindNone Kind = iota
	KindMain
	KindWorktree
)

// Info describes a single git working tree on disk.
type Info struct {
	// Path is the absolute path of the working tree (the directory the
	// user would `cd` into).
	Path string

	// Kind is Main or Worktree.
	Kind Kind

	// GitDir is the absolute path of the directory storing this
	// working tree's git metadata. For a main repo this is `<path>/.git`;
	// for a worktree this is `<main>/.git/worktrees/<name>`.
	GitDir string

	// MainRepoPath is the working-tree path of the main repo. For a
	// main repo this equals Path. For a worktree it is the directory
	// containing the main `.git/`.
	MainRepoPath string

	// Branch is the symbolic ref short name (e.g. "main"), or a short
	// SHA prefix when HEAD is detached, or "" if we couldn't read it.
	Branch string

	// HeadMTime is the last-modified time of HEAD. We use it as a
	// cheap "recently touched" proxy — checking out, committing, and
	// rebasing all rewrite HEAD, and it's free to stat.
	HeadMTime time.Time
}

// Inspect returns the working-tree Info at dir, or KindNone if dir is
// not a git working tree. It never returns an error for "not a repo" —
// the caller distinguishes via the returned Kind.
func Inspect(dir string) (Info, error) {
	gitPath := filepath.Join(dir, ".git")
	st, err := os.Lstat(gitPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Info{Kind: KindNone}, nil
	}
	if err != nil {
		return Info{}, err
	}

	switch {
	case st.IsDir():
		return inspectMain(dir, gitPath)
	case st.Mode().IsRegular():
		return inspectWorktree(dir, gitPath)
	default:
		// Symlink to a .git dir is unusual; treat as not-a-repo.
		return Info{Kind: KindNone}, nil
	}
}

func inspectMain(dir, gitDir string) (Info, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Info{}, err
	}
	headPath := filepath.Join(gitDir, "HEAD")
	branch, mtime := readHead(headPath)
	return Info{
		Path:         abs,
		Kind:         KindMain,
		GitDir:       gitDir,
		MainRepoPath: abs,
		Branch:       branch,
		HeadMTime:    mtime,
	}, nil
}

func inspectWorktree(dir, gitFile string) (Info, error) {
	gitDir, err := readGitdirFile(gitFile)
	if err != nil {
		return Info{}, fmt.Errorf("gitmeta: %s: %w", gitFile, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Info{}, err
	}
	main := mainRepoFromWorktreeGitdir(gitDir)
	headPath := filepath.Join(gitDir, "HEAD")
	branch, mtime := readHead(headPath)
	return Info{
		Path:         abs,
		Kind:         KindWorktree,
		GitDir:       gitDir,
		MainRepoPath: main,
		Branch:       branch,
		HeadMTime:    mtime,
	}, nil
}

// readGitdirFile parses a `.git` file as written by `git worktree add`.
// The format is a single line: `gitdir: /abs/path/to/.git/worktrees/<name>`.
func readGitdirFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if v, ok := strings.CutPrefix(line, "gitdir:"); ok {
			gd := strings.TrimSpace(v)
			if gd == "" {
				return "", errors.New("empty gitdir")
			}
			// gitdir paths are usually absolute, but guard anyway.
			if !filepath.IsAbs(gd) {
				gd = filepath.Join(filepath.Dir(path), gd)
			}
			return filepath.Clean(gd), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("no gitdir entry")
}

// mainRepoFromWorktreeGitdir derives the main repo's working tree from
// a worktree gitdir. The path looks like:
//
//	/.../<main>/.git/worktrees/<name>
//
// so we walk up two levels to land at `.git`, then up once more.
func mainRepoFromWorktreeGitdir(gitDir string) string {
	worktrees := filepath.Dir(gitDir) // .../.git/worktrees
	dotGit := filepath.Dir(worktrees) // .../.git
	mainRepo := filepath.Dir(dotGit)  // .../<main>
	if base := filepath.Base(dotGit); base != ".git" {
		// Layout we don't recognize — fall back to gitDir's parent so
		// callers still have something useful.
		return filepath.Dir(gitDir)
	}
	return mainRepo
}

// readHead returns the short branch name (or detached-SHA prefix) from
// HEAD plus its mtime. Errors are swallowed: a missing HEAD shouldn't
// stop a project from showing up in the picker.
func readHead(path string) (branch string, mtime time.Time) {
	st, err := os.Stat(path)
	if err == nil {
		mtime = st.ModTime()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", mtime
	}
	line := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(line, "ref:"); ok {
		// Symbolic ref form: "ref: refs/heads/main".
		ref = strings.TrimSpace(ref)
		ref = strings.TrimPrefix(ref, "refs/heads/")
		return ref, mtime
	}
	// Detached HEAD — the file holds a SHA.
	if len(line) >= 7 {
		return line[:7], mtime
	}
	return line, mtime
}

// LinkedWorktreeGitdirs returns the per-worktree gitdirs that live
// under `<mainRepo>/.git/worktrees/`. Each one corresponds to a single
// linked worktree.
func LinkedWorktreeGitdirs(mainRepo string) ([]string, error) {
	dir := filepath.Join(mainRepo, ".git", "worktrees")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out, nil
}

// WorktreePathFromGitdir resolves a per-worktree gitdir back to the
// working-tree directory the user would `cd` into.
//
// The gitdir contains a plain text file named `gitdir` whose single
// line is the absolute path of the worktree's `.git` regular file
// (e.g. `/path/to/worktree/.git`). The working tree is the parent of
// that. Note this is a *different* format from the worktree's own
// `.git` file, which uses `gitdir: <path>`.
func WorktreePathFromGitdir(gitdir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(gitdir, "gitdir"))
	if err != nil {
		return "", err
	}
	dotGitFile := strings.TrimSpace(string(raw))
	if dotGitFile == "" {
		return "", errors.New("empty worktree gitdir pointer")
	}
	return filepath.Dir(dotGitFile), nil
}

// LinkedWorktreePaths returns the working-tree paths of every linked
// worktree of mainRepo. Stale gitdirs (the working tree was deleted)
// are silently skipped.
func LinkedWorktreePaths(mainRepo string) ([]string, error) {
	dirs, err := LinkedWorktreeGitdirs(mainRepo)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		wt, err := WorktreePathFromGitdir(d)
		if err != nil {
			continue
		}
		// Skip if the worktree dir no longer exists on disk.
		if st, err := os.Stat(wt); err != nil || !st.IsDir() {
			continue
		}
		out = append(out, wt)
	}
	return out, nil
}
