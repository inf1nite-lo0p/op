// Package scanner walks the configured roots and finds every git
// working tree underneath, including linked worktrees.
//
// We deliberately avoid `exec.Command("git", ...)` — parsing on-disk
// state is far cheaper than forking. The walker is parallel: each root
// is handled by its own goroutine, and we use a small worker pool to
// inspect candidate directories.
package scanner

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/inf1nite-lo0p/op/internal/gitmeta"
)

// Project is the scanner's output type. It mirrors the cache schema in
// shape but is the in-memory representation used at runtime.
type Project struct {
	Path         string
	Name         string
	IsWorktree   bool
	MainRepoPath string
	Branch       string
	HeadMTime    int64 // Unix nanos. Easier to compare and serialize than time.Time.
}

// Options controls the behaviour of Scan.
type Options struct {
	// Roots are absolute paths to scan.
	Roots []string

	// Prune is a set of base names to skip while walking. Anything
	// matching short-circuits before we descend.
	Prune map[string]struct{}

	// MaxDepth caps how deep we walk relative to each root. 0 means
	// unlimited. We use a generous default in production.
	MaxDepth int

	// Workers is the number of inspection goroutines. <=0 picks a
	// reasonable default based on GOMAXPROCS.
	Workers int

	// OnFound is called once per project successfully classified.
	// The callback runs on a worker goroutine, so it must be safe
	// for concurrent invocation (typically just an atomic counter
	// increment for progress display).
	OnFound func()
}

// PruneSet builds the set used in Options from a slice — convenience
// for callers who load it from config.
func PruneSet(names []string) map[string]struct{} {
	s := make(map[string]struct{}, len(names))
	for _, n := range names {
		s[n] = struct{}{}
	}
	return s
}

// Scan walks every root and returns the discovered projects, sorted by
// most-recently-touched first. The context cancels both the walk and
// any in-flight inspections.
func Scan(ctx context.Context, opts Options) ([]Project, error) {
	if opts.Workers <= 0 {
		opts.Workers = 8
	}
	if opts.Prune == nil {
		opts.Prune = map[string]struct{}{}
	}

	// Channel of paths to inspect. The walker pushes; workers pop.
	jobs := make(chan string, 256)
	results := make(chan gitmeta.Info, 256)

	// Start workers first so the walker doesn't block on a full chan.
	var workerWg sync.WaitGroup
	for i := 0; i < opts.Workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for dir := range jobs {
				if ctx.Err() != nil {
					return
				}
				info, err := gitmeta.Inspect(dir)
				if err != nil || info.Kind == gitmeta.KindNone {
					continue
				}
				if opts.OnFound != nil {
					opts.OnFound()
				}
				results <- info
			}
		}()
	}

	// Walk all roots concurrently.
	var walkWg sync.WaitGroup
	for _, root := range opts.Roots {
		walkWg.Add(1)
		go func(root string) {
			defer walkWg.Done()
			walkRoot(ctx, root, opts, jobs)
		}(root)
	}

	// Closer goroutine: when all walkers are done, close jobs;
	// when all workers drain, close results. Sequencing channels
	// this way is the standard Go pattern.
	go func() {
		walkWg.Wait()
		close(jobs)
		workerWg.Wait()
		close(results)
	}()

	var infos []gitmeta.Info
	for r := range results {
		infos = append(infos, r)
	}
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		return nil, err
	}

	return toProjects(infos), nil
}

func walkRoot(ctx context.Context, root string, opts Options, jobs chan<- string) {
	rootClean := filepath.Clean(root)

	// We use filepath.WalkDir but skip subtrees once we land in or
	// under a `.git/`. We also enforce prune + max depth here.
	_ = filepath.WalkDir(rootClean, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		if err != nil {
			// Permission errors etc. — skip the offender, keep walking.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			// We only care about directories (and the `.git` regular
			// file is checked indirectly when we visit its parent).
			return nil
		}

		base := d.Name()

		// `.git` is always skipped — it's git's internal storage,
		// never a project we'd want to surface, and walking into it
		// just wastes time. Special-cased here so users don't have
		// to add it to their prune list manually.
		if base == ".git" {
			return filepath.SkipDir
		}

		// Depth cap, measured relative to the root.
		if opts.MaxDepth > 0 {
			rel, _ := filepath.Rel(rootClean, path)
			if rel != "." && depthOf(rel) > opts.MaxDepth {
				return filepath.SkipDir
			}
		}

		// Prune list — base name match.
		if _, pruned := opts.Prune[base]; pruned {
			return filepath.SkipDir
		}

		// If this directory itself is a working tree, queue it for
		// inspection.
		if kind := probe(path); kind != gitmeta.KindNone {
			select {
			case jobs <- path:
			case <-ctx.Done():
				return filepath.SkipAll
			}
			if kind == gitmeta.KindMain {
				// Enumerate linked worktrees explicitly. They usually
				// live under `<main>/.git/worktrees/...` (which we
				// skip via the `.git` filter above), so the walker
				// alone wouldn't find them.
				if wts, err := gitmeta.LinkedWorktreePaths(path); err == nil {
					for _, wt := range wts {
						select {
						case jobs <- wt:
						case <-ctx.Done():
							return filepath.SkipAll
						}
					}
				}
				// Keep descending into a main repo. Some users
				// keep "container" repos with independent project
				// repos as children (e.g. ~/projects/work/<repo>/
				// where ~/projects/work itself is also a repo).
				// Vendored deps are kept out by the prune list
				// (node_modules, vendor, target, etc.).
				return nil
			}
			// Worktrees usually don't contain more repos — stop here.
			return filepath.SkipDir
		}

		return nil
	})
}

// probe is a cheap "is this a working tree, and which kind?" check.
// One lstat instead of a full Inspect; the worker does the real parse.
func probe(dir string) gitmeta.Kind {
	st, err := os.Lstat(filepath.Join(dir, ".git"))
	if err != nil {
		return gitmeta.KindNone
	}
	switch {
	case st.IsDir():
		return gitmeta.KindMain
	case st.Mode().IsRegular():
		return gitmeta.KindWorktree
	default:
		return gitmeta.KindNone
	}
}

func depthOf(rel string) int {
	if rel == "" || rel == "." {
		return 0
	}
	d := 1
	for _, r := range rel {
		if r == filepath.Separator {
			d++
		}
	}
	return d
}

// toProjects converts gitmeta.Info into the scanner's Project type and
// sorts results by most-recently-touched HEAD. Worktrees of the same
// main repo are listed *immediately after* their main, so the TUI can
// render them grouped without having to re-sort.
//
// We dedupe by absolute path on the way in: a single worktree can be
// discovered both by the filesystem walker (its `.git` file) and by
// the main repo's linked-worktree expansion. Either path is correct;
// we just don't want it twice.
func toProjects(infos []gitmeta.Info) []Project {
	seen := make(map[string]struct{}, len(infos))
	projects := make([]Project, 0, len(infos))
	for _, in := range infos {
		if _, dup := seen[in.Path]; dup {
			continue
		}
		seen[in.Path] = struct{}{}
		projects = append(projects, Project{
			Path:         in.Path,
			Name:         filepath.Base(in.Path),
			IsWorktree:   in.Kind == gitmeta.KindWorktree,
			MainRepoPath: in.MainRepoPath,
			Branch:       in.Branch,
			HeadMTime:    in.HeadMTime.UnixNano(),
		})
	}

	// Step 1: sort everything by mtime desc (newest first).
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].HeadMTime > projects[j].HeadMTime
	})

	// Step 2: re-cluster so each main is followed by its worktrees.
	// We bucket by MainRepoPath, preserving the global mtime order
	// inside each bucket. The bucket's slot in the output is the
	// position of its newest member — usually the main, but can be
	// a worktree if the user touched a feature branch more recently.
	type bucket struct {
		key     string
		members []Project
	}
	order := make([]*bucket, 0)
	byKey := make(map[string]*bucket)
	for _, p := range projects {
		b, ok := byKey[p.MainRepoPath]
		if !ok {
			b = &bucket{key: p.MainRepoPath}
			byKey[p.MainRepoPath] = b
			order = append(order, b)
		}
		b.members = append(b.members, p)
	}

	out := make([]Project, 0, len(projects))
	for _, b := range order {
		// Within a bucket, put the main first if present, then
		// worktrees in mtime order.
		sort.SliceStable(b.members, func(i, j int) bool {
			if b.members[i].IsWorktree != b.members[j].IsWorktree {
				return !b.members[i].IsWorktree
			}
			return b.members[i].HeadMTime > b.members[j].HeadMTime
		})
		out = append(out, b.members...)
	}
	return out
}
