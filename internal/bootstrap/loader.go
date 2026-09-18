package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var defaultReferences = []string{"AGENTS.md", "SOUL.md", "USER.md"}

// Section is one labelled Bootstrap source copied into a startup snapshot.
type Section struct {
	Reference string
	Content   string
}

// Snapshot is an immutable collection of ordered Bootstrap sections.
type Snapshot struct {
	sections []Section
}

// NewSnapshot copies sections into an immutable startup value.
func NewSnapshot(sections []Section) Snapshot {
	return Snapshot{sections: append([]Section(nil), sections...)}
}

// Sections returns an independent copy in configured order.
func (snapshot Snapshot) Sections() []Section {
	return append([]Section(nil), snapshot.sections...)
}

// Loader reads Bootstrap files beneath an OryxOS workspace root.
type Loader struct{}

// BootstrapLoader is the explicit architecture name for Loader.
type BootstrapLoader = Loader

// Load returns an immutable startup snapshot. An empty reference list uses the
// three defaults, whose files may be absent and therefore contribute empty text.
func (Loader) Load(workspaceRoot string, references []string) (Snapshot, error) {
	root, err := canonicalRoot(workspaceRoot)
	if err != nil {
		return Snapshot{}, fmt.Errorf("load bootstrap: %w", err)
	}
	defaulted := len(references) == 0
	if defaulted {
		references = defaultReferences
	}
	sections := make([]Section, 0, len(references))
	for _, reference := range references {
		path, err := confinedExistingPath(root, reference)
		if err != nil {
			if defaulted && os.IsNotExist(err) {
				sections = append(sections, Section{Reference: reference})
				continue
			}
			return Snapshot{}, fmt.Errorf("load bootstrap %q: %w", reference, err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("load bootstrap %q: %w", reference, err)
		}
		sections = append(sections, Section{Reference: reference, Content: string(content)})
	}
	return NewSnapshot(sections), nil
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("workspace root is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	return canonical, nil
}

func confinedExistingPath(root, reference string) (string, error) {
	if strings.TrimSpace(reference) == "" || filepath.IsAbs(reference) {
		return "", fmt.Errorf("reference must be a non-empty workspace-relative path")
	}
	cleaned := filepath.Clean(reference)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reference escapes workspace")
	}
	candidate := filepath.Join(root, cleaned)
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, canonical)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("reference escapes workspace")
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
