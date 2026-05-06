package gitmeta

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeMain creates a minimal main repo at dir with HEAD pointing at branch.
// Returns the absolute path of the main working tree.
func makeMain(t *testing.T, dir, branch string) string {
	t.Helper()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "worktrees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(gitDir, "HEAD"),
		[]byte("ref: refs/heads/"+branch+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(dir)
	return abs
}

// makeWorktree creates a worktree linked to a main repo: a `.git` file in
// the worktree dir + the corresponding gitdir entry under the main's .git.
func makeWorktree(t *testing.T, mainPath, wtPath, name, branch string) {
	t.Helper()
	wtGitDir := filepath.Join(mainPath, ".git", "worktrees", name)
	if err := os.MkdirAll(wtGitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(wtGitDir, "HEAD"),
		[]byte("ref: refs/heads/"+branch+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(wtPath, ".git"),
		[]byte("gitdir: "+wtGitDir+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
}

func TestInspectMainRepo(t *testing.T) {
	dir := t.TempDir()
	main := makeMain(t, dir, "main")

	info, err := Inspect(dir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Kind != KindMain {
		t.Fatalf("kind = %v, want main", info.Kind)
	}
	if info.Branch != "main" {
		t.Fatalf("branch = %q, want main", info.Branch)
	}
	if info.Path != main {
		t.Fatalf("path = %q, want %q", info.Path, main)
	}
	if info.MainRepoPath != main {
		t.Fatalf("main repo = %q, want %q", info.MainRepoPath, main)
	}
	if info.HeadMTime.IsZero() {
		t.Fatal("head mtime should be set")
	}
}

func TestInspectWorktree(t *testing.T) {
	tmp := t.TempDir()
	mainDir := filepath.Join(tmp, "kit")
	wtDir := filepath.Join(tmp, "kit-feat")
	main := makeMain(t, mainDir, "main")
	makeWorktree(t, main, wtDir, "feat", "feat-x")

	info, err := Inspect(wtDir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Kind != KindWorktree {
		t.Fatalf("kind = %v, want worktree", info.Kind)
	}
	if info.Branch != "feat-x" {
		t.Fatalf("branch = %q, want feat-x", info.Branch)
	}
	if info.MainRepoPath != main {
		t.Fatalf("main repo = %q, want %q", info.MainRepoPath, main)
	}
}

func TestInspectMissing(t *testing.T) {
	info, err := Inspect(t.TempDir())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Kind != KindNone {
		t.Fatalf("kind = %v, want none", info.Kind)
	}
}

func TestInspectDetachedHead(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sha := "abcdef0123456789abcdef0123456789abcdef01"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(sha+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(dir)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if info.Branch != "abcdef0" {
		t.Fatalf("branch = %q, want short SHA", info.Branch)
	}
}

func TestLinkedWorktreeGitdirs(t *testing.T) {
	tmp := t.TempDir()
	mainDir := filepath.Join(tmp, "kit")
	main := makeMain(t, mainDir, "main")
	makeWorktree(t, main, filepath.Join(tmp, "kit-a"), "a", "a")
	makeWorktree(t, main, filepath.Join(tmp, "kit-b"), "b", "b")

	got, err := LinkedWorktreeGitdirs(main)
	if err != nil {
		t.Fatalf("LinkedWorktreeGitdirs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d worktree gitdirs, want 2: %v", len(got), got)
	}
}

func TestLinkedWorktreePaths(t *testing.T) {
	tmp := t.TempDir()
	mainDir := filepath.Join(tmp, "kit")
	main := makeMain(t, mainDir, "main")
	wtA := filepath.Join(tmp, "kit-a")
	makeWorktree(t, main, wtA, "a", "a")
	// Add the back-pointer so we can resolve the working tree path.
	wtGitDir := filepath.Join(main, ".git", "worktrees", "a")
	if err := os.WriteFile(filepath.Join(wtGitDir, "gitdir"), []byte(filepath.Join(wtA, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := LinkedWorktreePaths(main)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != wtA {
		t.Fatalf("got %v, want [%s]", got, wtA)
	}
}

func TestHeadMTimeReflectsWrite(t *testing.T) {
	dir := t.TempDir()
	makeMain(t, dir, "main")
	headPath := filepath.Join(dir, ".git", "HEAD")

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(headPath, old, old); err != nil {
		t.Fatal(err)
	}
	info, err := Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Allow for filesystem rounding.
	if info.HeadMTime.After(old.Add(time.Second)) {
		t.Fatalf("mtime not preserved: %v vs %v", info.HeadMTime, old)
	}
}
