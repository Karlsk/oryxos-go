package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/Karlsk/oryxos-go/internal/observability"
)

// Component is an application dependency with explicit start and close phases.
// Start may block only until the component is ready to serve; Close must honor its context.
type Component interface {
	Start(context.Context) error
	Close(context.Context) error
}

// TerminalSource reports a component's non-normal serving error.
type TerminalSource interface {
	Errors() <-chan error
}

// SignalContextFactory creates the root lifecycle context. Production uses OS signals; tests
// inject a controlled context factory to avoid installing process-wide signal handlers.
type SignalContextFactory func(context.Context) (context.Context, context.CancelFunc)

// Application owns assembled components and coordinates their lifecycle.
type Application struct {
	shutdownTimeout time.Duration
	observer        observability.Observer
	logger          *slog.Logger
	components      []Component
	signalContext   SignalContextFactory

	mu      sync.Mutex
	started []Component

	shutdownOnce sync.Once
	shutdownErr  error
}

// NewApplication constructs an application using OS signal handling at its process boundary.
func NewApplication(shutdownTimeout time.Duration, observer observability.Observer, logger *slog.Logger, components ...Component) (*Application, error) {
	if shutdownTimeout <= 0 {
		return nil, fmt.Errorf("shutdown timeout must be greater than zero")
	}
	return newApplication(shutdownTimeout, observer, logger, signalContext, components...), nil
}

func newApplication(shutdownTimeout time.Duration, observer observability.Observer, logger *slog.Logger, signalFactory SignalContextFactory, components ...Component) *Application {
	if signalFactory == nil {
		signalFactory = signalContext
	}
	return &Application{
		shutdownTimeout: shutdownTimeout,
		observer:        observer,
		logger:          logger,
		components:      append([]Component(nil), components...),
		signalContext:   signalFactory,
	}
}

// Run starts components in registration order, waits for cancellation or a terminal error, and
// closes the successful start prefix once in reverse order.
func (a *Application) Run(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	root, stop := a.signalContext(parent)
	defer stop()

	for index, component := range a.components {
		if component == nil {
			stop()
			return a.shutdown(fmt.Errorf("start component %d: nil component", index))
		}
		if err := component.Start(root); err != nil {
			rootCanceled := root.Err() != nil
			stop()
			if rootCanceled && errors.Is(err, context.Canceled) {
				return a.shutdown(nil)
			}
			return a.shutdown(fmt.Errorf("start component %d (%T): %w", index, component, err))
		}
		a.recordStarted(component)
	}

	if a.observer != nil {
		a.observer.SetReady(true)
	}
	trigger := a.waitForTermination(root)
	stop()
	return a.shutdown(trigger)
}

func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}

func (a *Application) recordStarted(component Component) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.started = append(a.started, component)
}

func (a *Application) startedComponents() []Component {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Component(nil), a.started...)
}

func (a *Application) waitForTermination(root context.Context) error {
	cases := []reflect.SelectCase{{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(root.Done())}}
	sources := []Component{nil}
	for _, component := range a.components {
		source, ok := component.(TerminalSource)
		if !ok {
			continue
		}
		errorsChannel := source.Errors()
		if errorsChannel == nil {
			continue
		}
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(errorsChannel)})
		sources = append(sources, component)
	}

	for {
		chosen, value, open := reflect.Select(cases)
		if chosen == 0 {
			return nil
		}
		if !open {
			cases[chosen].Chan = reflect.Value{}
			continue
		}
		terminalErr, _ := value.Interface().(error)
		if terminalErr == nil || errors.Is(terminalErr, http.ErrServerClosed) {
			continue
		}
		return fmt.Errorf("terminal component %T: %w", sources[chosen], terminalErr)
	}
}

func (a *Application) shutdown(trigger error) error {
	a.shutdownOnce.Do(func() {
		if a.observer != nil {
			a.observer.SetReady(false)
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
		defer cancel()

		errs := make([]error, 0, len(a.components)+2)
		if trigger != nil {
			errs = append(errs, trigger)
		}
		started := a.startedComponents()
		for index := len(started) - 1; index >= 0; index-- {
			component := started[index]
			if err := component.Close(shutdownCtx); err != nil {
				errs = append(errs, fmt.Errorf("close component %d (%T): %w", index, component, err))
			}
		}
		if err := shutdownCtx.Err(); err != nil {
			errs = append(errs, err)
		}
		a.shutdownErr = errors.Join(errs...)
		if a.shutdownErr != nil && a.logger != nil {
			a.logger.Error("app.shutdown_failed", "error_kind", "shutdown", "error", a.shutdownErr)
		}
	})
	return a.shutdownErr
}
