package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/app"
	"github.com/spf13/cobra"
)

func TestRootCommandHasExactlyTwelveLeaves(t *testing.T) {
	root := NewRootCommand(CommandDependencies{WorkspaceRoot: t.TempDir()})
	got := commandLeaves(root)
	want := []string{
		"chat [--profile <name>]",
		"gateway",
		"init",
		"profile create <name>",
		"profile delete <name>",
		"profile list",
		"profile show <name>",
		"provider list",
		"serve",
		"session list",
		"status",
		"tool list",
	}
	if len(got) != 12 {
		t.Fatalf("leaf count = %d, want 12: %v", len(got), got)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("leaves = %v, want %v", got, want)
	}
	for _, use := range []string{"profile", "provider", "tool", "session"} {
		if contains(got, use) {
			t.Fatalf("group command %q was registered as a leaf", use)
		}
	}
}

//nolint:gocyclo,revive // This integration contract deliberately covers the complete CLI lifecycle.
func TestFoundationCLIBehavior(t *testing.T) {
	workspaceRoot := t.TempDir()
	//nolint:noctx // Test-owned loopback listener is injected into the application seam.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind test listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ready := make(chan struct{})
	parent, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	newCommand := func(output io.Writer) *CommandDependencies {
		return &CommandDependencies{
			WorkspaceRoot: workspaceRoot,
			FoundationOptions: app.FoundationOptions{
				LogWriter: io.Discard,
				ListenerFactory: func(_, address string) (net.Listener, error) {
					close(ready)
					return listener, nil
				},
				SignalContextFactory: context.WithCancel,
			},
			Output: output,
		}
	}

	var initOutput bytes.Buffer
	if err := executeCommand(NewRootCommand(*newCommand(&initOutput)), parent, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	for _, target := range expectedWorkspaceTargets() {
		if !strings.Contains(initOutput.String(), "created "+target) {
			t.Fatalf("init output does not report created %q: %q", target, initOutput.String())
		}
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, ".oryxos", "USER.md"), []byte("keep this"), 0o600); err != nil {
		t.Fatalf("write USER.md sentinel: %v", err)
	}
	var secondInitOutput bytes.Buffer
	if err := executeCommand(NewRootCommand(*newCommand(&secondInitOutput)), parent, "init"); err != nil {
		t.Fatalf("second init error = %v", err)
	}
	for _, target := range expectedWorkspaceTargets() {
		if !strings.Contains(secondInitOutput.String(), "skipped "+target) {
			t.Fatalf("second init output does not report skipped %q: %q", target, secondInitOutput.String())
		}
	}
	if got, err := os.ReadFile(filepath.Join(workspaceRoot, ".oryxos", "USER.md")); err != nil || string(got) != "keep this" {
		t.Fatalf("USER.md after second init = %q, %v; want sentinel unchanged", got, err)
	}

	var statusOutput bytes.Buffer
	if err := executeCommand(NewRootCommand(*newCommand(&statusOutput)), parent, "status"); err != nil {
		t.Fatalf("status error = %v", err)
	}
	if got, want := statusOutput.String(), "mode: foundation\nworkspace: initialized\n"; got != want {
		t.Fatalf("status output = %q, want %q", got, want)
	}

	var serveOutput bytes.Buffer
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- executeCommand(NewRootCommand(*newCommand(&serveOutput)), parent, "serve")
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("serve did not become ready")
	}
	for _, path := range []string{"/api/v1/health", "/api/v1/info"} {
		requestContext, cancelRequest := context.WithTimeout(context.Background(), time.Second)
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "http://"+listener.Addr().String()+path, nil)
		if err != nil {
			cancelRequest()
			t.Fatalf("construct %s request: %v", path, err)
		}
		response, err := http.DefaultClient.Do(request)
		cancelRequest()
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		var envelope struct {
			Code      string          `json:"code"`
			RequestID string          `json:"request_id"`
			Data      json.RawMessage `json:"data"`
		}
		err = json.NewDecoder(response.Body).Decode(&envelope)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || envelope.Code != "ok" || envelope.RequestID == "" || len(envelope.Data) == 0 {
			t.Fatalf("%s response = status %d envelope %#v decode %v, want valid success Result", path, response.StatusCode, envelope, err)
		}
	}
	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve command did not exit after cancellation")
	}

	for _, args := range [][]string{
		{"chat"}, {"gateway"}, {"profile", "list"}, {"profile", "create", "demo"},
		{"profile", "show", "demo"}, {"profile", "delete", "demo"}, {"provider", "list"},
		{"tool", "list"}, {"session", "list"},
	} {
		var output bytes.Buffer
		err := executeCommand(NewRootCommand(*newCommand(&output)), context.Background(), args...)
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("%q error = %v, want ErrNotImplemented", args, err)
		}
		if output.Len() != 0 {
			t.Fatalf("%q output = %q, want no fabricated data", args, output.String())
		}
	}
}

func TestChatDeferredInvocationContract(t *testing.T) {
	for _, args := range [][]string{{"chat"}, {"chat", "--profile", "default"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var output bytes.Buffer
			err := executeCommand(NewRootCommand(CommandDependencies{Output: &output}), context.Background(), args...)
			if !errors.Is(err, ErrNotImplemented) {
				t.Fatalf("%q error = %v, want ErrNotImplemented", args, err)
			}
			if output.Len() != 0 {
				t.Fatalf("%q output = %q, want no fabricated data", args, output.String())
			}
		})
	}

	var output bytes.Buffer
	err := executeCommand(NewRootCommand(CommandDependencies{Output: &output}), context.Background(), "chat", "unexpected")
	if err == nil || errors.Is(err, ErrNotImplemented) {
		t.Fatalf("chat positional error = %v, want Cobra argument validation error", err)
	}
	if output.Len() != 0 {
		t.Fatalf("chat positional output = %q, want no fabricated data", output.String())
	}
}

func commandLeaves(root *cobra.Command) []string {
	var leaves []string
	var walk func(*cobra.Command, []string)
	walk = func(command *cobra.Command, parents []string) {
		commands := command.Commands()
		if len(commands) == 0 {
			leaves = append(leaves, strings.Join(append(parents, command.Use), " "))
			return
		}
		for _, child := range commands {
			walk(child, append(parents, command.Use))
		}
	}
	for _, child := range root.Commands() {
		walk(child, nil)
	}
	sort.Strings(leaves)
	return leaves
}

//nolint:revive // Cobra command ownership is the test helper's primary parameter.
func executeCommand(root *cobra.Command, ctx context.Context, args ...string) error {
	root.SetArgs(args)
	root.SetContext(ctx)
	return root.Execute()
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
