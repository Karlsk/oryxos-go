package bootstrap

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoaderUsesDefaultOrderAndEmptyMissingDefaults(t *testing.T) {
	root := t.TempDir()
	writeBootstrap(t, root, "AGENTS.md", "project")
	writeBootstrap(t, root, "USER.md", "user")

	snapshot, err := (Loader{}).Load(root, nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []Section{{Reference: "AGENTS.md", Content: "project"}, {Reference: "SOUL.md", Content: ""}, {Reference: "USER.md", Content: "user"}}
	if !reflect.DeepEqual(snapshot.Sections(), want) {
		t.Fatalf("Sections() = %#v, want %#v", snapshot.Sections(), want)
	}
}

func TestLoaderExplicitOrderMissingConfinementAndSnapshot(t *testing.T) {
	root := t.TempDir()
	path := writeBootstrap(t, root, "SECOND.md", "second")
	writeBootstrap(t, root, "FIRST.md", "first")
	snapshot, err := (Loader{}).Load(root, []string{"SECOND.md", "FIRST.md"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := snapshot.Sections(); got[0].Reference != "SECOND.md" || got[1].Reference != "FIRST.md" {
		t.Fatalf("Sections() = %#v, want explicit order", got)
	}
	if err := os.WriteFile(path, []byte("changed"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	sections := snapshot.Sections()
	sections[0].Content = "mutated"
	if snapshot.Sections()[0].Content != "second" {
		t.Fatalf("snapshot changed after disk/caller mutation: %#v", snapshot.Sections())
	}
	for _, references := range [][]string{{"MISSING.md"}, {"../escape.md"}, {filepath.Join(root, "FIRST.md")}} {
		if _, err := (Loader{}).Load(root, references); err == nil {
			t.Fatalf("Load(%v) error = nil, want failure", references)
		}
	}
}

func TestLoaderRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "LINK.md")); err != nil {
		t.Fatalf("create confinement symlink: %v", err)
	}
	if _, err := (Loader{}).Load(root, []string{"LINK.md"}); err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("Load(symlink) error = %v, want confinement failure", err)
	}
}

func writeBootstrap(t *testing.T, root, reference, content string) string {
	t.Helper()
	path := filepath.Join(root, reference)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write bootstrap: %v", err)
	}
	return path
}
