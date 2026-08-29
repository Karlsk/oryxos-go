package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/Karlsk/oryxos-go/internal/observability"
)

func TestApplicationNormalCancellation(t *testing.T) {
	observer := observability.NewObserver()
	first := newFakeComponent("A")
	second := newFakeComponent("B")
	application := newTestApplication(observer, time.Second, first, second)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	first.waitStarted(t)
	second.waitStarted(t)
	cancel()

	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	assertEvents(t, first, second, []string{"start:A", "start:B", "close:B", "close:A"})
	for _, component := range []*fakeComponent{first, second} {
		component.assertCloseContext(t)
	}
	if observer.Snapshot().Ready {
		t.Fatal("observer remained ready after shutdown")
	}
}

func TestApplicationStartFailure(t *testing.T) {
	startB := errors.New("start B")
	first := newFakeComponent("A")
	second := newFakeComponent("B")
	second.startErr = startB
	third := newFakeComponent("C")
	application := newTestApplication(observability.NewObserver(), time.Second, first, second, third)

	err := application.Run(context.Background())
	if !errors.Is(err, startB) {
		t.Fatalf("Run() error = %v, want errors.Is(start B)", err)
	}
	assertEvents(t, first, second, third, []string{"start:A", "start:B", "close:A"})
	if third.startCount() != 0 {
		t.Fatalf("C.Start calls = %d, want 0", third.startCount())
	}
	if first.closeCount() != 1 {
		t.Fatalf("A.Close calls = %d, want 1", first.closeCount())
	}
}

func TestApplicationStartFailureCancelsRootWorkerBeforeClose(t *testing.T) {
	startB := errors.New("start B")
	first := newFakeComponent("A")
	first.ownsWorker = true
	second := newFakeComponent("B")
	second.startErr = startB
	signalFactory, cancelRoot := newControlledSignalContext()
	application := newTestApplicationWithSignalFactory(observability.NewObserver(), time.Second, signalFactory, first, second)

	runDone := runApplication(application, context.Background())
	first.waitStarted(t)
	second.waitStarted(t)

	err := awaitFailureRun(t, runDone, cancelRoot)
	if !errors.Is(err, startB) {
		t.Fatalf("Run() error = %v, want errors.Is(start B)", err)
	}
	assertWorkerExited(t, first)
}

func TestApplicationServeFailure(t *testing.T) {
	serveFailed := errors.New("serve failed")
	first := newFakeComponent("A")
	second := newFakeComponent("B")
	second.terminal = make(chan error, 1)
	application := newTestApplication(observability.NewObserver(), time.Second, first, second)

	runDone := runApplication(application, context.Background())
	first.waitStarted(t)
	second.waitStarted(t)
	second.terminal <- serveFailed

	err := <-runDone
	if !errors.Is(err, serveFailed) {
		t.Fatalf("Run() error = %v, want errors.Is(serve failed)", err)
	}
	assertEvents(t, first, second, []string{"start:A", "start:B", "close:B", "close:A"})
	if first.closeCount() != 1 || second.closeCount() != 1 {
		t.Fatalf("close calls = A:%d B:%d, want one each", first.closeCount(), second.closeCount())
	}
}

func TestApplicationTerminalFailureCancelsRootWorkerBeforeClose(t *testing.T) {
	serveFailed := errors.New("serve failed")
	first := newFakeComponent("A")
	first.ownsWorker = true
	second := newFakeComponent("B")
	second.terminal = make(chan error, 1)
	signalFactory, cancelRoot := newControlledSignalContext()
	application := newTestApplicationWithSignalFactory(observability.NewObserver(), time.Second, signalFactory, first, second)

	runDone := runApplication(application, context.Background())
	first.waitStarted(t)
	second.waitStarted(t)
	second.terminal <- serveFailed

	err := awaitFailureRun(t, runDone, cancelRoot)
	if !errors.Is(err, serveFailed) {
		t.Fatalf("Run() error = %v, want errors.Is(serve failed)", err)
	}
	assertWorkerExited(t, first)
}

func TestApplicationJoinsCloseErrors(t *testing.T) {
	closeA := errors.New("close A")
	closeB := errors.New("close B")
	first := newFakeComponent("A")
	first.closeErr = closeA
	second := newFakeComponent("B")
	second.closeErr = closeB
	application := newTestApplication(observability.NewObserver(), time.Second, first, second)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	first.waitStarted(t)
	second.waitStarted(t)
	cancel()

	err := <-runDone
	if !errors.Is(err, closeA) || !errors.Is(err, closeB) {
		t.Fatalf("Run() error = %v, want both close errors", err)
	}
	assertEvents(t, first, second, []string{"start:A", "start:B", "close:B", "close:A"})
}

func TestApplicationShutdownDeadline(t *testing.T) {
	component := newFakeComponent("A")
	component.waitForCloseContext = true
	application := newTestApplication(observability.NewObserver(), 20*time.Millisecond, component)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	component.waitStarted(t)
	cancel()

	err := <-runDone
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want errors.Is(context.DeadlineExceeded)", err)
	}
	component.assertDeadlineExceededContext(t)
}

func TestApplicationOwnedGoroutineExits(t *testing.T) {
	component := newFakeComponent("A")
	component.ownsWorker = true
	application := newTestApplication(observability.NewObserver(), time.Second, component)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	component.waitStarted(t)
	cancel()

	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	select {
	case <-component.workerDone:
	default:
		t.Fatal("owned worker had not exited before Run returned")
	}
}

func TestApplicationServerClosedIsNormal(t *testing.T) {
	component := newFakeComponent("server")
	component.terminal = make(chan error, 1)
	application := newTestApplication(observability.NewObserver(), time.Second, component)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	component.waitStarted(t)
	cancel()
	component.terminal <- http.ErrServerClosed

	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v, want nil when server closes during shutdown", err)
	}
}

func TestApplicationShutdownIsIdempotent(t *testing.T) {
	component := newFakeComponent("A")
	application := newTestApplication(observability.NewObserver(), time.Second, component)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	component.waitStarted(t)
	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if err := application.shutdown(nil); err != nil {
		t.Fatalf("first repeated shutdown error = %v", err)
	}
	if err := application.shutdown(nil); err != nil {
		t.Fatalf("second repeated shutdown error = %v", err)
	}
	if component.closeCount() != 1 {
		t.Fatalf("Close calls = %d, want 1", component.closeCount())
	}
}

func TestNewApplicationRejectsNonPositiveShutdownTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Nanosecond} {
		t.Run(timeout.String(), func(t *testing.T) {
			application, err := NewApplication(timeout, observability.NewObserver(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err == nil || application != nil {
				t.Fatalf("NewApplication(%s) = %#v, %v; want nil application and error", timeout, application, err)
			}
		})
	}
}

func TestApplicationLogsStableShutdownFailureEvent(t *testing.T) {
	const secret = "shutdown-secret"
	var output bytes.Buffer
	component := newFakeComponent("A")
	component.eventLog = &eventRecorder{}
	component.closeErr = errors.New("upstream rejected Bearer " + secret)
	application := newApplication(
		time.Second,
		observability.NewObserver(),
		observability.NewLogger(&output, slog.LevelInfo),
		testSignalContext,
		component,
	)

	parent, cancel := context.WithCancel(context.Background())
	runDone := runApplication(application, parent)
	component.waitStarted(t)
	cancel()
	if err := <-runDone; err == nil {
		t.Fatal("Run() error = nil, want close failure")
	}
	if bytes.Contains(output.Bytes(), []byte(secret)) {
		t.Fatalf("shutdown log contains secret: %s", output.Bytes())
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatalf("unmarshal shutdown log: %v", err)
	}
	if got := record["msg"]; got != "app.shutdown_failed" {
		t.Fatalf("shutdown event = %v, want app.shutdown_failed", got)
	}
	if got := record["error_kind"]; got != "shutdown" {
		t.Fatalf("error_kind = %v, want shutdown", got)
	}
	if got := record["error"]; got != "[REDACTED]" {
		t.Fatalf("error = %v, want [REDACTED]", got)
	}
}

func TestApplicationLifecycleTable(t *testing.T) {
	cases := []struct {
		name string
		run  func(*testing.T)
	}{
		{"normal_cancellation", TestApplicationNormalCancellation},
		{"start_failure", TestApplicationStartFailure},
		{"serve_failure", TestApplicationServeFailure},
		{"multiple_close_errors", TestApplicationJoinsCloseErrors},
		{"shutdown_deadline", TestApplicationShutdownDeadline},
		{"owned_goroutine_exits", TestApplicationOwnedGoroutineExits},
		{"server_closed_is_normal", TestApplicationServerClosedIsNormal},
		{"shutdown_is_idempotent", TestApplicationShutdownIsIdempotent},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, testCase.run)
	}
}

func newTestApplication(observer observability.Observer, timeout time.Duration, components ...Component) *Application {
	return newTestApplicationWithSignalFactory(observer, timeout, testSignalContext, components...)
}

func newTestApplicationWithSignalFactory(observer observability.Observer, timeout time.Duration, signalFactory SignalContextFactory, components ...Component) *Application {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := &eventRecorder{}
	for _, component := range components {
		if fake, ok := component.(*fakeComponent); ok {
			fake.eventLog = events
		}
	}
	return newApplication(timeout, observer, logger, signalFactory, components...)
}

func testSignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

func newControlledSignalContext() (SignalContextFactory, context.CancelFunc) {
	root, cancel := context.WithCancel(context.Background())
	return func(context.Context) (context.Context, context.CancelFunc) {
		return root, cancel
	}, cancel
}

//nolint:revive // Application is the helper's subject under test.
func runApplication(application *Application, parent context.Context) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- application.Run(parent)
	}()
	return done
}

func awaitFailureRun(t *testing.T, runDone <-chan error, cancelRoot context.CancelFunc) error {
	t.Helper()
	select {
	case err := <-runDone:
		return err
	case <-time.After(time.Second):
		cancelRoot()
		<-runDone
		t.Fatal("Run() blocked while Close waited for a root-context worker")
		return nil
	}
}

func assertWorkerExited(t *testing.T, component *fakeComponent) {
	t.Helper()
	select {
	case <-component.workerDone:
	default:
		t.Fatal("owned worker had not exited before Run returned")
	}
}

type fakeComponent struct {
	name                string
	startErr            error
	closeErr            error
	terminal            chan error
	waitForCloseContext bool
	ownsWorker          bool
	started             chan struct{}
	workerDone          chan struct{}
	eventLog            *eventRecorder

	mu                 sync.Mutex
	startCalls         int
	closeCalls         int
	closeContextErrors []error
	closeHasDeadline   []bool
}

func newFakeComponent(name string) *fakeComponent {
	return &fakeComponent{
		name:       name,
		started:    make(chan struct{}),
		workerDone: make(chan struct{}),
	}
}

func (f *fakeComponent) Start(ctx context.Context) error {
	f.mu.Lock()
	f.startCalls++
	f.mu.Unlock()
	f.eventLog.record("start:" + f.name)
	close(f.started)
	if f.startErr != nil {
		return f.startErr
	}
	if f.ownsWorker {
		go func() {
			<-ctx.Done()
			close(f.workerDone)
		}()
	}
	return nil
}

func (f *fakeComponent) Close(ctx context.Context) error {
	f.mu.Lock()
	f.closeCalls++
	_, hasDeadline := ctx.Deadline()
	f.closeContextErrors = append(f.closeContextErrors, ctx.Err())
	f.closeHasDeadline = append(f.closeHasDeadline, hasDeadline)
	f.mu.Unlock()
	f.eventLog.record("close:" + f.name)
	if f.ownsWorker {
		<-f.workerDone
	}
	if f.waitForCloseContext {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.closeErr
}

func (f *fakeComponent) Errors() <-chan error { return f.terminal }

func (f *fakeComponent) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatalf("%s did not start", f.name)
	}
}

func (f *fakeComponent) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCalls
}

func (f *fakeComponent) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeCalls
}

func (f *fakeComponent) assertCloseContext(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.closeContextErrors) != 1 {
		t.Fatalf("%s close contexts = %d, want 1", f.name, len(f.closeContextErrors))
	}
	if err := f.closeContextErrors[0]; err != nil {
		t.Fatalf("%s Close context error = %v, want active context", f.name, err)
	}
	if !f.closeHasDeadline[0] {
		t.Fatalf("%s Close context has no deadline", f.name)
	}
}

func (f *fakeComponent) assertDeadlineExceededContext(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.closeContextErrors) != 1 {
		t.Fatalf("%s close contexts = %d, want 1", f.name, len(f.closeContextErrors))
	}
	if !f.closeHasDeadline[0] {
		t.Fatalf("%s Close context has no deadline", f.name)
	}
	if err := f.closeContextErrors[0]; err != nil {
		t.Fatalf("%s Close context error = %v, want active context at entry", f.name, err)
	}
}

type eventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *eventRecorder) record(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *eventRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

func assertEvents(t *testing.T, components ...any) {
	t.Helper()
	want, ok := components[len(components)-1].([]string)
	if !ok {
		t.Fatal("assertEvents requires the expected event sequence")
	}
	for _, item := range components[:len(components)-1] {
		component, ok := item.(*fakeComponent)
		if !ok {
			t.Fatal("assertEvents requires fake components")
		}
		if component.eventLog == nil {
			t.Fatal("assertEvents requires components with an event recorder")
		}
	}
	first := components[0].(*fakeComponent)
	got := first.eventLog.snapshot()
	if len(got) != len(want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("events = %v, want %v", got, want)
		}
	}
}
