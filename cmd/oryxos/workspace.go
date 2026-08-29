package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

var workspaceDirectories = []string{"profiles", "sessions", "skills", "logs", "memory"}

var workspaceFiles = map[string]string{
	"AGENTS.md":             "# Project Instructions\n",
	"SOUL.md":               "# Agent Personality\n",
	"USER.md":               "# User Preferences\n",
	"memory/MEMORY.md":      "",
	"mcp_servers.yaml":      "servers: []\n",
	"profiles/default.yaml": defaultProfileYAML,
}

const defaultProfileYAML = `name: default
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

// WorkspaceState is the observable initialization state of a workspace.
type WorkspaceState string

const (
	WorkspaceInitialized    WorkspaceState = "initialized"
	WorkspaceNotInitialized WorkspaceState = "not_initialized"
)

// InitializeWorkspace exclusively creates missing foundation workspace targets without
// modifying targets that already exist.
func InitializeWorkspace(root string, out io.Writer) error {
	return initializeWorkspace(root, out, osWorkspaceFS{})
}

func initializeWorkspace(root string, out io.Writer, fs workspaceFS) error {
	workspace := filepath.Join(root, ".oryxos")
	if _, err := ensureWorkspaceDirectory(workspace, fs); err != nil {
		return fmt.Errorf("create workspace root: %w", err)
	}

	results := make(map[string]string, len(workspaceDirectories)+len(workspaceFiles))
	for _, directory := range workspaceDirectories {
		created, err := ensureWorkspaceDirectory(filepath.Join(workspace, directory), fs)
		if err != nil {
			return fmt.Errorf("create workspace directory %s: %w", directory, err)
		}
		results[directory] = creationResult(created)
	}

	filePaths := make([]string, 0, len(workspaceFiles))
	for path := range workspaceFiles {
		filePaths = append(filePaths, path)
	}
	sort.Strings(filePaths)
	for _, path := range filePaths {
		created, err := createWorkspaceFile(filepath.Join(workspace, filepath.FromSlash(path)), workspaceFiles[path], fs)
		if err != nil {
			return fmt.Errorf("create workspace file %s: %w", path, err)
		}
		results[path] = creationResult(created)
	}

	targets := make([]string, 0, len(results))
	for target := range results {
		targets = append(targets, target)
	}
	sort.Strings(targets)
	for _, target := range targets {
		if _, err := fmt.Fprintf(out, "%s %s\n", results[target], target); err != nil {
			return err
		}
	}
	return nil
}

// WorkspaceStatus reports initialized only when all required workspace targets exist with the
// expected file or directory shape.
func WorkspaceStatus(root string) WorkspaceState {
	workspace := filepath.Join(root, ".oryxos")
	if !isWorkspaceDirectory(workspace) {
		return WorkspaceNotInitialized
	}
	for _, directory := range workspaceDirectories {
		if !isWorkspaceDirectory(filepath.Join(workspace, directory)) {
			return WorkspaceNotInitialized
		}
	}
	for path := range workspaceFiles {
		if !isWorkspaceFile(filepath.Join(workspace, filepath.FromSlash(path))) {
			return WorkspaceNotInitialized
		}
	}
	return WorkspaceInitialized
}

type workspaceFile interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

type workspaceFS interface {
	Lstat(string) (os.FileInfo, error)
	Mkdir(string, os.FileMode) error
	Chmod(string, os.FileMode) error
	CreateTemp(string, string) (workspaceFile, error)
	Link(string, string) error
	Remove(string) error
}

type osWorkspaceFS struct{}

func (osWorkspaceFS) Lstat(path string) (os.FileInfo, error) { return os.Lstat(path) }
func (osWorkspaceFS) Mkdir(path string, mode os.FileMode) error {
	return os.Mkdir(path, mode)
}
func (osWorkspaceFS) Chmod(path string, mode os.FileMode) error { return os.Chmod(path, mode) }
func (osWorkspaceFS) CreateTemp(directory, pattern string) (workspaceFile, error) {
	return os.CreateTemp(directory, pattern)
}
func (osWorkspaceFS) Link(oldPath, newPath string) error { return os.Link(oldPath, newPath) }
func (osWorkspaceFS) Remove(path string) error           { return os.Remove(path) }

func ensureWorkspaceDirectory(path string, fs workspaceFS) (bool, error) {
	info, err := fs.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("existing path is a symlink")
		}
		if !info.IsDir() {
			return false, fmt.Errorf("existing path is not a directory")
		}
		if info.Mode().Perm() != 0o750 {
			return false, fmt.Errorf("existing directory permissions are %04o, want 0750", info.Mode().Perm())
		}
		return false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := fs.Mkdir(path, 0o750); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ensureWorkspaceDirectory(path, fs)
		}
		return false, err
	}
	// #nosec G302 -- this is a workspace directory that must allow group traversal.
	if err := fs.Chmod(path, 0o750); err != nil {
		cleanupErr := fs.Remove(path)
		return false, errors.Join(err, cleanupError("remove directory after chmod failure", cleanupErr))
	}
	return true, nil
}

func createWorkspaceFile(path, content string, fs workspaceFS) (bool, error) {
	exists, err := existingWorkspaceFile(path, fs)
	if err != nil || exists {
		return false, err
	}

	file, err := fs.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, err
	}
	tempPath := file.Name()
	cleanupBeforePublication := func(operationErr error, closeFile bool) error {
		if closeFile {
			operationErr = errors.Join(operationErr, cleanupError("close temporary file", file.Close()))
		}
		return errors.Join(operationErr, cleanupError("remove temporary file", fs.Remove(tempPath)))
	}

	if err := file.Chmod(0o600); err != nil {
		return false, cleanupBeforePublication(err, true)
	}
	if err := writeWorkspaceContent(file, []byte(content)); err != nil {
		return false, cleanupBeforePublication(err, true)
	}
	if err := file.Sync(); err != nil {
		return false, cleanupBeforePublication(err, true)
	}
	if err := file.Close(); err != nil {
		return false, errors.Join(err, cleanupError("remove temporary file", fs.Remove(tempPath)))
	}

	if err := fs.Link(tempPath, path); err != nil {
		cleanupErr := cleanupError("remove temporary file", fs.Remove(tempPath))
		if errors.Is(err, os.ErrExist) {
			exists, validationErr := existingWorkspaceFile(path, fs)
			if validationErr == nil && !exists {
				validationErr = fmt.Errorf("publication target disappeared")
			}
			return false, errors.Join(validationErr, cleanupErr)
		}
		return false, errors.Join(err, cleanupErr)
	}
	if err := fs.Remove(tempPath); err != nil {
		return true, fmt.Errorf("remove published temporary file: %w", err)
	}
	return true, nil
}

func existingWorkspaceFile(path string, fs workspaceFS) (bool, error) {
	info, err := fs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("existing path is a symlink")
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("existing path is not a regular file")
	}
	return true, nil
}

func writeWorkspaceContent(file io.Writer, content []byte) error {
	for len(content) > 0 {
		written, err := file.Write(content)
		if written < 0 || written > len(content) {
			return io.ErrShortWrite
		}
		content = content[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func cleanupError(operation string, err error) error {
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func isWorkspaceDirectory(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() && info.Mode().Perm() == 0o750
}

func isWorkspaceFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular()
}

func creationResult(created bool) string {
	if created {
		return "created"
	}
	return "skipped"
}
