package firstrun

import (
	"reflect"
	"testing"

	"github.com/inf1nite-lo0p/op/internal/config"
)

func TestCfgFromInputBlankUsesDefault(t *testing.T) {
	got := cfgFromInput("")
	want := config.Defaults()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("blank input should yield Defaults: got %#v, want %#v", got, want)
	}
}

func TestCfgFromInputWhitespaceUsesDefault(t *testing.T) {
	got := cfgFromInput("   \t  ")
	want := config.Defaults()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("whitespace input should yield Defaults: got %#v, want %#v", got, want)
	}
}

func TestCfgFromInputCommaWhitespaceUsesDefault(t *testing.T) {
	got := cfgFromInput(", , ,")
	want := config.Defaults()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("comma-only input should yield Defaults: got %#v, want %#v", got, want)
	}
}

func TestCfgFromInputSingleRoot(t *testing.T) {
	got := cfgFromInput("~/code")
	if !reflect.DeepEqual(got.Roots, []string{"~/code"}) {
		t.Fatalf("got roots %v, want [~/code]", got.Roots)
	}
}

func TestCfgFromInputMultipleRoots(t *testing.T) {
	got := cfgFromInput("~/code, ~/work,~/repos")
	want := []string{"~/code", "~/work", "~/repos"}
	if !reflect.DeepEqual(got.Roots, want) {
		t.Fatalf("got roots %v, want %v", got.Roots, want)
	}
}

func TestCfgFromInputPreservesPruneList(t *testing.T) {
	// User customises roots; prune stays at the comprehensive
	// default list so output / vendored deps are still skipped.
	got := cfgFromInput("/abs/path")
	if len(got.Prune) == 0 {
		t.Fatal("prune list should be preserved from Defaults()")
	}
}
