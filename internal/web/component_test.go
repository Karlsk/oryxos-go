package web

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/observability"
)

//nolint:noctx,revive // The test verifies the injected listener seam and context-owned shutdown.
func TestServerGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind loopback listener: %v", err)
	}
	bound := make(chan struct{})
	server := NewServer(testServerConfig(), observability.NewObserver(), testLogger(io.Discard), "test-version", func(network, address string) (net.Listener, error) {
		close(bound)
		return listener, nil
	})

	startErrors := make(chan error, 1)
	go func() {
		startErrors <- server.Start(context.Background())
	}()
	select {
	case <-bound:
	case <-time.After(time.Second):
		t.Fatal("server did not bind the injected listener")
	}
	if err := <-startErrors; err != nil {
		t.Fatalf("start server: %v", err)
	}

	requestContext, cancelRequest := context.WithTimeout(context.Background(), time.Second)
	defer cancelRequest()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, "http://"+listener.Addr().String()+"/api/v1/health", nil)
	if err != nil {
		t.Fatalf("construct request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request served listener: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}

	closeContext, cancelClose := context.WithTimeout(context.Background(), time.Second)
	defer cancelClose()
	if err := server.Close(closeContext); err != nil {
		t.Fatalf("close server: %v", err)
	}
	if err := server.Close(closeContext); err != nil {
		t.Fatalf("repeat close server: %v", err)
	}
	select {
	case err := <-server.Errors():
		if !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("terminal server error = %v, want none or ErrServerClosed", err)
		}
	default:
	}
}
