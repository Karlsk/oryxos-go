package app

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
)

func TestNewFoundationRejectsInvalidConfiguration(t *testing.T) {
	_, err := NewFoundation(FoundationOptions{
		ServerYAML: []byte("shutdown_timeout: 0s\n"),
		LogWriter:  io.Discard,
	})
	if err == nil {
		t.Fatal("NewFoundation() error = nil, want invalid server configuration")
	}
}

func TestNewFoundationReturnsListenerStartFailure(t *testing.T) {
	listenFailed := errors.New("listen failed")
	application, err := NewFoundation(FoundationOptions{
		LogWriter:            io.Discard,
		ListenerFactory:      func(string, string) (net.Listener, error) { return nil, listenFailed },
		SignalContextFactory: testSignalContext,
	})
	if err != nil {
		t.Fatalf("NewFoundation() error = %v, want nil", err)
	}

	err = application.Run(context.Background())
	if !errors.Is(err, listenFailed) {
		t.Fatalf("Run() error = %v, want errors.Is(listen failed)", err)
	}
}
