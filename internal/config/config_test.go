package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadFromCreatesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "op", "config.toml")

	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom returned error: %v", err)
	}
	if !reflect.DeepEqual(got, Defaults()) {
		t.Fatalf("expected defaults, got %#v", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file written to %s: %v", path, err)
	}
}

func TestLoadFromParsesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	contents := `
roots = ["/a", "/b"]
prune = ["x"]
`
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	want := Config{Roots: []string{"/a", "/b"}, Prune: []string{"x"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestLoadFromRejectsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("this is = not [ valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFrom(path); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestExpandHome(t *testing.T) {
	home := "/home/test"
	cases := []struct {
		in, want string
	}{
		{"~", home},
		{"~/projects", "/home/test/projects"},
		{"/abs/path", "/abs/path"},
		{"relative", "relative"},
		// Only leading-tilde is special — embedded ~ stays.
		{"/foo/~/bar", "/foo/~/bar"},
	}
	for _, c := range cases {
		if got := expandHome(c.in, home); got != c.want {
			t.Errorf("expandHome(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSetFieldVimMode(t *testing.T) {
	cases := []struct {
		input string
		want  bool
		ok    bool
	}{
		{"true", true, true},
		{"on", true, true},
		{"yes", true, true},
		{"1", true, true},
		{"false", false, true},
		{"off", false, true},
		{"no", false, true},
		{"0", false, true},
		{"True", true, true}, // case-insensitive
		{"OFF", false, true},
		{"yep", false, false}, // bad value
		{"", false, false},
	}
	for _, c := range cases {
		got, err := SetField(Config{}, "vim_mode", c.input)
		if (err == nil) != c.ok {
			t.Errorf("SetField(vim_mode, %q): err=%v, want ok=%v", c.input, err, c.ok)
			continue
		}
		if c.ok && got.VimMode != c.want {
			t.Errorf("SetField(vim_mode, %q): VimMode = %v, want %v", c.input, got.VimMode, c.want)
		}
	}
}

func TestSetFieldUnknownKeyErrors(t *testing.T) {
	if _, err := SetField(Config{}, "nope", "true"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestGetField(t *testing.T) {
	c := Config{Roots: []string{"/a", "/b"}, Prune: []string{"x"}, VimMode: true}
	if v, _ := GetField(c, "vim_mode"); v != "true" {
		t.Errorf("vim_mode = %q, want true", v)
	}
	if v, _ := GetField(c, "roots"); v != "/a\n/b" {
		t.Errorf("roots = %q, want /a\\n/b", v)
	}
	if _, err := GetField(c, "nope"); err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestExpandRootsDedupes(t *testing.T) {
	c := Config{Roots: []string{"/a", "/a", "", "  ", "/b"}}
	got, err := c.ExpandRoots()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/a", "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
