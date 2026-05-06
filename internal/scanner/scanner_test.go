package scanner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

// makeMain creates a fake main repo at dir with a HEAD pointing at branch.
func makeMain(t *testing.T, dir, branch string) {
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
}

// makeWorktree creates a worktree pointing at a main repo's gitdir.
// This mirrors the on-disk layout of `git worktree add`.
func makeWorktree(t *testing.T, mainDir, wtPath, name, branch string) {
	t.Helper()
	wtGitDir := filepath.Join(mainDir, ".git", "worktrees", name)
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
	// And the back-pointer: <main>/.git/worktrees/<name>/gitdir holds
	// the absolute path of the worktree's `.git` file.
	if err := os.WriteFile(
		filepath.Join(wtGitDir, "gitdir"),
		[]byte(filepath.Join(wtPath, ".git")+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
}

func TestScanFindsMainsAndWorktrees(t *testing.T) {
	root := t.TempDir()
	// /root/projects/kit (main)
	// /root/projects/kit-feat (worktree of kit)
	// /root/projects/api (main)
	// /root/projects/api/node_modules (pruned)
	kit := filepath.Join(root, "projects", "kit")
	api := filepath.Join(root, "projects", "api")
	wt := filepath.Join(root, "projects", "kit-feat")

	if err := os.MkdirAll(kit, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(api, 0o755); err != nil {
		t.Fatal(err)
	}
	makeMain(t, kit, "main")
	makeMain(t, api, "main")
	makeWorktree(t, kit, wt, "feat", "feat-x")

	// Drop a node_modules with a .git inside to confirm pruning.
	nm := filepath.Join(api, "node_modules", "trap")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	makeMain(t, nm, "trap")

	got, err := Scan(context.Background(), Options{
		Roots: []string{root},
		Prune: PruneSet([]string{"node_modules"}),
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d projects, want 3 (kit, api, kit-feat): %+v", len(got), got)
	}

	// Worktree must be grouped right after its main repo.
	for i, p := range got {
		if p.IsWorktree {
			if i == 0 {
				t.Fatalf("worktree first in output, expected to follow its main")
			}
			if got[i-1].Path != p.MainRepoPath {
				t.Fatalf("worktree not grouped: prev=%s main=%s", got[i-1].Path, p.MainRepoPath)
			}
		}
	}
}

func TestScanRespectsMaxDepth(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d", "deep")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	makeMain(t, deep, "main")

	got, err := Scan(context.Background(), Options{
		Roots:    []string{root},
		MaxDepth: 2,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected depth limit to hide deep repo, got %v", got)
	}
}

func TestScanFindsWorktreesNestedInsideMainRepo(t *testing.T) {
	// Mirrors the user's real layout: worktrees live at
	// <main>/.claude/worktrees/<branch>/. Without explicit handling
	// the walker SkipDir's after seeing the main repo and never
	// descends into its tree.
	root := t.TempDir()
	main := filepath.Join(root, "kit")
	wt := filepath.Join(main, ".claude", "worktrees", "feat-x")

	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	makeMain(t, main, "main")
	makeWorktree(t, main, wt, "feat-x", "feat-x")

	got, err := Scan(context.Background(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d projects, want main+worktree: %+v", len(got), got)
	}
	var sawWT bool
	for _, p := range got {
		if p.IsWorktree {
			sawWT = true
			if p.Path != wt {
				t.Fatalf("worktree path = %s, want %s", p.Path, wt)
			}
			if p.Branch != "feat-x" {
				t.Fatalf("worktree branch = %s, want feat-x", p.Branch)
			}
		}
	}
	if !sawWT {
		t.Fatal("expected a worktree row")
	}
}

func TestScanOnFoundCallbackFiresOncePerProject(t *testing.T) {
	// The picker uses OnFound to drive its live "scanning… N found"
	// counter. The callback must fire exactly once for every project
	// that ends up in the result set, no more and no less.
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		makeMain(t, dir, "main")
	}

	var found atomic.Int64
	got, err := Scan(context.Background(), Options{
		Roots:   []string{root},
		OnFound: func() { found.Add(1) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if int(found.Load()) != len(got) {
		t.Fatalf("OnFound fired %d times, want %d (= len(results))", found.Load(), len(got))
	}
	if len(got) != 4 {
		t.Fatalf("got %d results, want 4", len(got))
	}
}

func TestScanFindsNestedReposInsideContainerRepo(t *testing.T) {
	// Real-world layout the user has at ~/projects/prezly/new:
	// the directory itself is a git repo AND has many independent
	// git repos as children. Both the parent and every child must
	// show up in the picker.
	root := t.TempDir()
	parent := filepath.Join(root, "container")
	makeMain(t, parent, "main")

	for _, name := range []string{"alpha", "beta", "gamma"} {
		dir := filepath.Join(parent, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		makeMain(t, dir, "main")
	}

	got, err := Scan(context.Background(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		names := make([]string, len(got))
		for i, p := range got {
			names[i] = p.Name
		}
		t.Fatalf("got %d projects, want 4 (container + 3 children): %v", len(got), names)
	}
}

func TestScanStillSkipsDotGit(t *testing.T) {
	// Even though we descend into main repos now, we must never walk
	// into `.git/` — it's git's storage, not project content.
	root := t.TempDir()
	main := filepath.Join(root, "repo")
	makeMain(t, main, "main")
	// Plant a fake .git/foo/bar that would be very expensive if walked.
	junk := filepath.Join(main, ".git", "foo", "bar", "baz")
	if err := os.MkdirAll(junk, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(context.Background(), Options{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d projects, want 1: %+v", len(got), got)
	}
}

func TestScanSkipsPrunedSubtreesEvenInsideRepos(t *testing.T) {
	// Now that the scanner descends into main repos, we lean on the
	// prune list to keep vendored / dependency repos out of the
	// picker. A repo nested under `node_modules/` (or any pruned
	// dir) must NOT show up.
	root := t.TempDir()
	repo := filepath.Join(root, "outer")
	makeMain(t, repo, "main")

	vendored := filepath.Join(repo, "node_modules", "some-package")
	if err := os.MkdirAll(vendored, 0o755); err != nil {
		t.Fatal(err)
	}
	makeMain(t, vendored, "main")

	got, err := Scan(context.Background(), Options{
		Roots: []string{root},
		Prune: PruneSet([]string{"node_modules"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d projects, want 1 (only outer; node_modules pruned): %+v", len(got), got)
	}
}
