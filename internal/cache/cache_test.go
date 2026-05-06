package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	want := []Entry{
		{
			Path: "/a", Name: "a", IsWorktree: false,
			MainRepoPath: "/a", Branch: "main",
			HeadMTime: time.Now().UTC().Truncate(time.Second),
		},
		{
			Path: "/a-feat", Name: "a-feat", IsWorktree: true,
			MainRepoPath: "/a", Branch: "feat",
			HeadMTime: time.Now().UTC().Truncate(time.Second),
		},
	}
	if err := saveTo(path, want); err != nil {
		t.Fatalf("saveTo: %v", err)
	}
	got, ok, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if !ok {
		t.Fatal("expected cache to load")
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Path != want[i].Path || got[i].Branch != want[i].Branch {
			t.Errorf("entry %d mismatch: %+v vs %+v", i, got[i], want[i])
		}
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	got, ok, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("expected (nil,false), got (%v,%v)", got, ok)
	}
}

func TestLoadVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	bad := File{Version: 999, Projects: []Entry{{Path: "/x", Name: "x"}}}
	data, _ := json.Marshal(bad)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("version mismatch should yield empty: got=%v ok=%v", got, ok)
	}
}

func TestLoadCorruptJSONReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := loadFrom(path)
	if ok || got != nil {
		t.Fatalf("expected empty load, got %v %v", got, ok)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	// Writing into the same dir as an existing cache should leave the
	// previous file intact until rename.
	dir := t.TempDir()
	path := filepath.Join(dir, "projects.json")

	original := []Entry{{Path: "/orig", Name: "orig"}}
	if err := saveTo(path, original); err != nil {
		t.Fatal(err)
	}
	// Sanity: cache dir contains exactly one file (no leftover tmp).
	listFiles := func() []string {
		entries, _ := os.ReadDir(dir)
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return out
	}
	if got := listFiles(); len(got) != 1 || got[0] != "projects.json" {
		t.Fatalf("unexpected files after save: %v", got)
	}

	// Replace and re-check.
	updated := []Entry{{Path: "/updated", Name: "updated"}}
	if err := saveTo(path, updated); err != nil {
		t.Fatal(err)
	}
	got, _, _ := loadFrom(path)
	if len(got) != 1 || got[0].Path != "/updated" {
		t.Fatalf("post-update load = %v, want /updated", got)
	}
	if files := listFiles(); len(files) != 1 {
		t.Fatalf("leftover files after atomic save: %v", files)
	}
}
