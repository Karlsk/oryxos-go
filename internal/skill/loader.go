package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Section is one labelled Skill copied into a startup snapshot.
type Section struct {
	Reference string
	Content   string
}

// Snapshot is an immutable ordered set of Profile-referenced Skills.
type Snapshot struct {
	sections []Section
}

// NewSnapshot copies sections into an immutable startup value.
func NewSnapshot(sections []Section) Snapshot {
	return Snapshot{sections: append([]Section(nil), sections...)}
}

// Sections returns an independent copy in Profile order.
func (snapshot Snapshot) Sections() []Section {
	return append([]Section(nil), snapshot.sections...)
}

// Loader reads explicitly referenced SKILL.md files beneath the Skills root.
type Loader struct{}

// SkillLoader is the explicit architecture name for Loader.
type SkillLoader = Loader

// Load copies only the supplied references and never discovers directories.
func (Loader) Load(skillsRoot string, references []string) (Snapshot, error) {
	root, err := canonicalRoot(skillsRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load skills: %w", err)
	}
	seen := make(map[string]struct{}, len(references))
	sections := make([]Section, 0, len(references))
	for _, reference := range references {
		cleaned := filepath.Clean(reference)
		if filepath.Base(cleaned) != "SKILL.md" {
			return Snapshot{}, fmt.Errorf("load skill %q: reference must resolve to SKILL.md", reference)
		}
		if _, duplicate := seen[cleaned]; duplicate {
			return Snapshot{}, fmt.Errorf("load skill %q: duplicate reference", reference)
		}
		seen[cleaned] = struct{}{}
		path, err := confinedExistingPath(root, reference)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load skill %q: %w", reference, err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load skill %q: %w", reference, err)
		}
		sections = append(sections, Section{Reference: reference, Content: string(content)})
	}
	return NewSnapshot(sections), nil
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("skills root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve skills root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve skills root: %w", err)
	}
	return canonical, nil
}

func confinedExistingPath(root, reference string) (string, error) {
	if strings.TrimSpace(reference) == "" || filepath.IsAbs(reference) {
		return "", fmt.Errorf("reference must be a non-empty relative path")
	}
	cleaned := filepath.Clean(reference)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reference escapes skills root")
	}
	candidate := filepath.Join(root, cleaned)
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, canonical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reference escapes skills root")
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("reference is not a regular file")
	}
	return canonical, nil
}
