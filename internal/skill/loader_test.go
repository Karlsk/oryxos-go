package skill

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoaderLoadsOnlyReferencedSkillsInOrderAsSnapshot(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "weather/SKILL.md", "weather")
	dailyPath := writeSkill(t, root, "daily/SKILL.md", "daily")
	writeSkill(t, root, "ignored/SKILL.md", "ignored")

	snapshot, err := (Loader{}).Load(root, []string{"daily/SKILL.md", "weather/SKILL.md"})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []Section{{Reference: "daily/SKILL.md", Content: "daily"}, {Reference: "weather/SKILL.md", Content: "weather"}}
	if !reflect.DeepEqual(snapshot.Sections(), want) {
		t.Fatalf("Sections() = %#v, want %#v", snapshot.Sections(), want)
	}
	if err := os.WriteFile(dailyPath, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	sections := snapshot.Sections()
	sections[0].Content = "mutated"
	if !reflect.DeepEqual(snapshot.Sections(), want) {
		t.Fatalf("snapshot is mutable: %#v", snapshot.Sections())
	}
}

func TestLoaderRejectsMissingDuplicateAndEscapingReferences(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "weather/SKILL.md", "weather")
	cases := [][]string{
		{"missing/SKILL.md"},
		{"weather/SKILL.md", "weather/SKILL.md"},
		{"../SKILL.md"},
		{"weather"},
		{filepath.Join(root, "weather/SKILL.md")},
	}
	for _, references := range cases {
		if _, err := (Loader{}).Load(root, references); err == nil {
			t.Errorf("Load(%v) error = nil, want failure", references)
		}
	}
}

func TestLoaderRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDirectory := t.TempDir()
	outside := filepath.Join(outsideDirectory, "SKILL.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkDirectory := filepath.Join(root, "linked")
	if err := os.Symlink(outsideDirectory, linkDirectory); err != nil {
		t.Fatalf("create confinement symlink: %v", err)
	}
	if _, err := (Loader{}).Load(root, []string{"linked/SKILL.md"}); err == nil {
		t.Fatal("Load(symlink escape) error = nil, want failure")
	}
}

func writeSkill(t *testing.T, root, reference, content string) string {
	t.Helper()
	path := filepath.Join(root, reference)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return path
}
