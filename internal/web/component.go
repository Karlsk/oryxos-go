package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// ListenerFactory binds a listener. It is replaceable by package tests.
type ListenerFactory func(network, address string) (net.Listener, error)

var (
	errServerStarted = errors.New("web server already started")
	errServerClosed  = errors.New("web server is closed")
)

// startState keeps serving ownership and shutdown transitions synchronized.
type startState struct {
	mu        sync.Mutex
	started   bool
	starting  bool
	closed    bool
	closeOnce sync.Once
	closeErr  error
	doneOnce  sync.Once
}

// Start synchronously binds then launches the one owned Serve goroutine.
func (s *Server) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	state := &s.state
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return errServerClosed
	}
	if state.started || state.starting {
		state.mu.Unlock()
		return errServerStarted
	}
	state.starting = true
	state.mu.Unlock()

	listener, err := s.listenerFactory("tcp", s.httpServer.Addr)
	if err != nil {
		state.mu.Lock()
		state.starting = false
		state.mu.Unlock()
		return fmt.Errorf("bind HTTP listener: %w", err)
	}

	state.mu.Lock()
	state.starting = false
	if state.closed {
		state.mu.Unlock()
		_ = listener.Close()
		s.finish()
		return errServerClosed
	}
	state.started = true
	state.mu.Unlock()

	go func() {
		defer s.finish()
		if serveErr := s.httpServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			s.errors <- serveErr
		}
	}()
	return nil
}

// Close gracefully stops serving and waits for the owned Serve goroutine.
func (s *Server) Close(ctx context.Context) error {
	state := &s.state
	state.closeOnce.Do(func() {
		state.closeErr = s.close(ctx, state)
	})
	return state.closeErr
}

// Errors reports non-normal terminal Serve failures.
func (s *Server) Errors() <-chan error {
	return s.errors
}

func (s *Server) close(ctx context.Context, state *startState) error {
	state.mu.Lock()
	state.closed = true
	started := state.started
	state.mu.Unlock()
	if !started {
		return nil
	}

	shutdownErr := s.httpServer.Shutdown(ctx)
	var closeErr error
	if shutdownErr != nil {
		closeErr = s.httpServer.Close()
	}
	<-s.done
	if errors.Is(closeErr, http.ErrServerClosed) {
		closeErr = nil
	}
	return errors.Join(shutdownErr, closeErr)
}

func (s *Server) finish() {
	s.state.doneOnce.Do(func() {
		close(s.done)
	})
}
