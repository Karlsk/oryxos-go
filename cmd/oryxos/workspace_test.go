package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestInitCreatesExactWorkspace(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	if err := InitializeWorkspace(root, &output); err != nil {
		t.Fatalf("InitializeWorkspace() error = %v", err)
	}
	workspace := filepath.Join(root, ".oryxos")
	for _, directory := range []string{"profiles", "sessions", "skills", "logs", "memory"} {
		info, err := os.Stat(filepath.Join(workspace, directory))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o750 {
			t.Fatalf("directory %s = %#v, %v; want directory mode 0750", directory, info, err)
		}
	}
	wantFiles := map[string]string{
		"AGENTS.md":             "# Project Instructions\n",
		"SOUL.md":               "# Agent Personality\n",
		"USER.md":               "# User Preferences\n",
		"memory/MEMORY.md":      "",
		"mcp_servers.yaml":      "servers: []\n",
		"profiles/default.yaml": expectedDefaultProfileFixture,
	}
	for path, want := range wantFiles {
		got, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(path)))
		if err != nil || string(got) != want {
			t.Fatalf("file %s = %q, %v; want %q", path, got, err, want)
		}
		info, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(path)))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %s = %#v, %v; want 0600", path, info, err)
		}
	}
	for _, forbidden := range []string{"agents", "tools", "sessions/oryxos.db"} {
		if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(forbidden))); !os.IsNotExist(err) {
			t.Fatalf("forbidden workspace path %q exists or stat error = %v", forbidden, err)
		}
	}
	gotDirectories, gotFiles := workspaceInventory(t, workspace)
	wantDirectories := []string{"logs", "memory", "profiles", "sessions", "skills"}
	wantFilePaths := []string{"AGENTS.md", "SOUL.md", "USER.md", "mcp_servers.yaml", "memory/MEMORY.md", "profiles/default.yaml"}
	if strings.Join(gotDirectories, "\n") != strings.Join(wantDirectories, "\n") {
		t.Fatalf("workspace directories = %v, want exact five %v", gotDirectories, wantDirectories)
	}
	if strings.Join(gotFiles, "\n") != strings.Join(wantFilePaths, "\n") {
		t.Fatalf("workspace files = %v, want exact six %v", gotFiles, wantFilePaths)
	}
	lines := nonEmptyLines(output.String())
	wantLines := make([]string, 0, len(expectedWorkspaceTargets()))
	for _, target := range expectedWorkspaceTargets() {
		wantLines = append(wantLines, "created "+target)
	}
	if strings.Join(lines, "\n") != strings.Join(wantLines, "\n") {
		t.Fatalf("init output = %q, want %q", lines, wantLines)
	}
}

func TestInitDoesNotPublishUnfinalizedFiles(t *testing.T) {
	for _, failure := range []string{"chmod", "write", "sync", "close"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			fault := errors.New(failure + " failed")
			fs := &faultWorkspaceFS{
				workspaceFS: osWorkspaceFS{},
				filePattern: "AGENTS.md",
				fileFailure: failure,
				fileErr:     fault,
			}

			err := initializeWorkspace(root, &bytes.Buffer{}, fs)
			if !errors.Is(err, fault) {
				t.Fatalf("initializeWorkspace() error = %v, want errors.Is(%v)", err, fault)
			}
			target := filepath.Join(root, ".oryxos", "AGENTS.md")
			if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("unfinalized target stat error = %v, want not exist", statErr)
			}
			if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
				t.Fatalf("WorkspaceStatus() = %q, want %q after %s failure", got, WorkspaceNotInitialized, failure)
			}

			if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
				t.Fatalf("repeated InitializeWorkspace() error = %v", err)
			}
			assertFinalizedWorkspaceFile(t, target, "# Project Instructions\n")
			if got := WorkspaceStatus(root); got != WorkspaceInitialized {
				t.Fatalf("WorkspaceStatus() = %q, want %q after repeated init", got, WorkspaceInitialized)
			}
		})
	}
}

func TestInitCleansNewDirectoryAfterChmodFailure(t *testing.T) {
	root := t.TempDir()
	chmodFailed := errors.New("directory chmod failed")
	fs := &faultWorkspaceFS{
		workspaceFS:        osWorkspaceFS{},
		directoryChmodBase: "profiles",
		directoryChmodErr:  chmodFailed,
	}

	err := initializeWorkspace(root, &bytes.Buffer{}, fs)
	if !errors.Is(err, chmodFailed) {
		t.Fatalf("initializeWorkspace() error = %v, want errors.Is(chmod failed)", err)
	}
	profiles := filepath.Join(root, ".oryxos", "profiles")
	if _, statErr := os.Lstat(profiles); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed directory stat error = %v, want removed", statErr)
	}
	if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q", got, WorkspaceNotInitialized)
	}
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("repeated InitializeWorkspace() error = %v", err)
	}
}

func TestInitDoesNotAcceptDirectoryAfterChmodAndCleanupFailure(t *testing.T) {
	root := t.TempDir()
	chmodFailed := errors.New("directory chmod failed")
	cleanupFailed := errors.New("directory cleanup failed")
	fs := &faultWorkspaceFS{
		workspaceFS:        osWorkspaceFS{},
		directoryChmodBase: "profiles",
		directoryChmodErr:  chmodFailed,
		directoryRemoveErr: cleanupFailed,
		poisonDirectory:    true,
	}

	err := initializeWorkspace(root, &bytes.Buffer{}, fs)
	if !errors.Is(err, chmodFailed) || !errors.Is(err, cleanupFailed) {
		t.Fatalf("initializeWorkspace() error = %v, want joined chmod and cleanup failures", err)
	}
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err == nil {
		t.Fatal("repeated InitializeWorkspace() error = nil, want poisoned directory rejection")
	}
	if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q after repeated init", got, WorkspaceNotInitialized)
	}
}

func TestInitPublicationFailureLeavesFinalTargetAbsent(t *testing.T) {
	root := t.TempDir()
	publishFailed := errors.New("publish failed")
	fs := &faultWorkspaceFS{workspaceFS: osWorkspaceFS{}, linkErr: publishFailed}

	err := initializeWorkspace(root, &bytes.Buffer{}, fs)
	if !errors.Is(err, publishFailed) {
		t.Fatalf("initializeWorkspace() error = %v, want errors.Is(publish failed)", err)
	}
	if _, statErr := os.Lstat(filepath.Join(root, ".oryxos", "AGENTS.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("unpublished target stat error = %v, want not exist", statErr)
	}
	if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q", got, WorkspaceNotInitialized)
	}
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("repeated InitializeWorkspace() error = %v", err)
	}
}

func TestInitCleanupFailureDoesNotPoisonPublishedTarget(t *testing.T) {
	root := t.TempDir()
	cleanupFailed := errors.New("cleanup failed")
	fs := &faultWorkspaceFS{
		workspaceFS:   osWorkspaceFS{},
		filePattern:   "AGENTS.md",
		removeTempErr: cleanupFailed,
	}

	err := initializeWorkspace(root, &bytes.Buffer{}, fs)
	if !errors.Is(err, cleanupFailed) {
		t.Fatalf("initializeWorkspace() error = %v, want errors.Is(cleanup failed)", err)
	}
	target := filepath.Join(root, ".oryxos", "AGENTS.md")
	assertFinalizedWorkspaceFile(t, target, "# Project Instructions\n")
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("repeated InitializeWorkspace() error = %v", err)
	}
	if got := WorkspaceStatus(root); got != WorkspaceInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q after repeated init", got, WorkspaceInitialized)
	}
}

func TestInitIsIdempotentAndNonDestructive(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("first InitializeWorkspace() error = %v", err)
	}
	userPath := filepath.Join(root, ".oryxos", "USER.md")
	if err := os.WriteFile(userPath, []byte("user-maintained sentinel\n"), 0o600); err != nil {
		t.Fatalf("write USER.md: %v", err)
	}
	var output bytes.Buffer
	if err := InitializeWorkspace(root, &output); err != nil {
		t.Fatalf("second InitializeWorkspace() error = %v", err)
	}
	for _, target := range expectedWorkspaceTargets() {
		if !strings.Contains(output.String(), "skipped "+target+"\n") {
			t.Fatalf("second output does not report skipped %q: %q", target, output.String())
		}
	}
	if got, err := os.ReadFile(userPath); err != nil || string(got) != "user-maintained sentinel\n" {
		t.Fatalf("USER.md = %q, %v; want sentinel unchanged", got, err)
	}
	for _, path := range []string{"AGENTS.md", "SOUL.md", "USER.md", "profiles/default.yaml", "memory/MEMORY.md", "mcp_servers.yaml"} {
		if _, err := os.Stat(filepath.Join(root, ".oryxos", filepath.FromSlash(path))); err != nil {
			t.Fatalf("workspace file %s absent after repeated init: %v", path, err)
		}
	}
}

func TestInitRejectsRequiredFileDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".oryxos", "AGENTS.md")
	if err := os.MkdirAll(target, 0o750); err != nil {
		t.Fatalf("create required-file directory: %v", err)
	}

	if err := InitializeWorkspace(root, &bytes.Buffer{}); err == nil {
		t.Fatal("InitializeWorkspace() error = nil, want required file wrong-type error")
	}
}

func TestInitRejectsRequiredFileSymlink(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("InitializeWorkspace() setup error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside sentinel\n"), 0o600); err != nil {
		t.Fatalf("write outside sentinel: %v", err)
	}
	target := filepath.Join(root, ".oryxos", "AGENTS.md")
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove required file: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("create required-file symlink: %v", err)
	}

	if err := InitializeWorkspace(root, &bytes.Buffer{}); err == nil {
		t.Fatal("InitializeWorkspace() error = nil, want required file symlink error")
	}
	if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q for required-file symlink", got, WorkspaceNotInitialized)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "outside sentinel\n" {
		t.Fatalf("outside file = %q, %v; want sentinel unchanged", got, err)
	}
}

func TestInitRejectsWorkspaceRootSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".oryxos")); err != nil {
		t.Fatalf("create workspace-root symlink: %v", err)
	}

	if err := InitializeWorkspace(root, &bytes.Buffer{}); err == nil {
		t.Fatal("InitializeWorkspace() error = nil, want workspace-root symlink error")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read outside directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory entries = %v, want no writes", entryNames(entries))
	}
}

func TestInitRejectsRequiredDirectorySymlinkWithoutOutsideWrite(t *testing.T) {
	root := t.TempDir()
	if err := InitializeWorkspace(root, &bytes.Buffer{}); err != nil {
		t.Fatalf("InitializeWorkspace() setup error = %v", err)
	}
	memory := filepath.Join(root, ".oryxos", "memory")
	if err := os.Remove(filepath.Join(memory, "MEMORY.md")); err != nil {
		t.Fatalf("remove memory file: %v", err)
	}
	if err := os.Remove(memory); err != nil {
		t.Fatalf("remove memory directory: %v", err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, memory); err != nil {
		t.Fatalf("create required-directory symlink: %v", err)
	}

	if err := InitializeWorkspace(root, &bytes.Buffer{}); err == nil {
		t.Fatal("InitializeWorkspace() error = nil, want required-directory symlink error")
	}
	if got := WorkspaceStatus(root); got != WorkspaceNotInitialized {
		t.Fatalf("WorkspaceStatus() = %q, want %q for required-directory symlink", got, WorkspaceNotInitialized)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatalf("read outside directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("outside directory entries = %v, want no writes", entryNames(entries))
	}
}

func expectedWorkspaceTargets() []string {
	targets := []string{
		"profiles", "sessions", "skills", "logs", "memory",
		"AGENTS.md", "SOUL.md", "USER.md", "memory/MEMORY.md", "mcp_servers.yaml", "profiles/default.yaml",
	}
	sort.Strings(targets)
	return targets
}

func nonEmptyLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

const expectedDefaultProfileFixture = `name: default
description: Default OryxOS profile

identity:
  agent_name: Oryx
  prompt: You are a helpful enterprise assistant.

provider:
  name: deepseek
  model: deepseek-chat
  api_key: ${LLM_API_KEY}
  base_url: ""
  temperature: 0.7

tools:
  - read_file
  - write_file
  - list_dir
  - shell
  - http_get
  - http_post
  - save_memory
  - recall_memory
  - notify

skills: []
mcp_servers: []
notify_channels: []
schedules: []
channels:
  - name: cli
    config: {}
bootstrap:
  - AGENTS.md
  - SOUL.md
  - USER.md
settings:
  max_iterations: 10
  max_history_turns: 20
`

func workspaceInventory(t *testing.T, workspace string) ([]string, []string) {
	t.Helper()
	var directories []string
	var files []string
	if err := filepath.WalkDir(workspace, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(workspace, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			directories = append(directories, relative)
		} else {
			files = append(files, relative)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk workspace: %v", err)
	}
	sort.Strings(directories)
	sort.Strings(files)
	return directories, files
}

func assertFinalizedWorkspaceFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("file %s = %q, %v; want %q", path, got, err, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode %s = %#v, %v; want 0600", path, info, err)
	}
}

type faultWorkspaceFS struct {
	workspaceFS
	filePattern        string
	fileFailure        string
	fileErr            error
	directoryChmodBase string
	directoryChmodErr  error
	directoryRemoveErr error
	poisonDirectory    bool
	linkErr            error
	removeTempErr      error
}

func (fs *faultWorkspaceFS) Chmod(path string, mode os.FileMode) error {
	if filepath.Base(path) == fs.directoryChmodBase {
		if fs.poisonDirectory {
			if err := fs.workspaceFS.Chmod(path, 0o700); err != nil {
				return errors.Join(fs.directoryChmodErr, err)
			}
		}
		return fs.directoryChmodErr
	}
	return fs.workspaceFS.Chmod(path, mode)
}

func (fs *faultWorkspaceFS) CreateTemp(directory, pattern string) (workspaceFile, error) {
	file, err := fs.workspaceFS.CreateTemp(directory, pattern)
	if err != nil {
		return nil, err
	}
	if fs.fileFailure != "" && strings.Contains(pattern, fs.filePattern) {
		return &faultWorkspaceFile{workspaceFile: file, failure: fs.fileFailure, err: fs.fileErr}, nil
	}
	return file, nil
}

func (fs *faultWorkspaceFS) Link(oldPath, newPath string) error {
	if fs.linkErr != nil {
		return fs.linkErr
	}
	return fs.workspaceFS.Link(oldPath, newPath)
}

func (fs *faultWorkspaceFS) Remove(path string) error {
	if fs.directoryRemoveErr != nil && filepath.Base(path) == fs.directoryChmodBase {
		err := fs.directoryRemoveErr
		fs.directoryRemoveErr = nil
		return err
	}
	if fs.removeTempErr != nil && strings.Contains(filepath.Base(path), ".tmp-") {
		err := fs.removeTempErr
		fs.removeTempErr = nil
		return err
	}
	return fs.workspaceFS.Remove(path)
}

type faultWorkspaceFile struct {
	workspaceFile
	failure string
	err     error
}

func (file *faultWorkspaceFile) Chmod(mode os.FileMode) error {
	if file.failure == "chmod" {
		return file.err
	}
	return file.workspaceFile.Chmod(mode)
}

func (file *faultWorkspaceFile) Write(content []byte) (int, error) {
	if file.failure == "write" {
		return 0, file.err
	}
	return file.workspaceFile.Write(content)
}

func (file *faultWorkspaceFile) Sync() error {
	if file.failure == "sync" {
		return file.err
	}
	return file.workspaceFile.Sync()
}

func (file *faultWorkspaceFile) Close() error {
	err := file.workspaceFile.Close()
	if file.failure == "close" {
		return errors.Join(err, file.err)
	}
	return err
}

var _ io.Writer = (*faultWorkspaceFile)(nil)
