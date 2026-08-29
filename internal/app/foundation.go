package app

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/observability"
	"github.com/Karlsk/oryxos-go/internal/web"
)

// FoundationOptions supplies process-boundary inputs and test seams for foundation assembly.
type FoundationOptions struct {
	ServerYAML           []byte
	LookupEnv            func(string) (string, bool)
	LogWriter            io.Writer
	Version              string
	ListenerFactory      web.ListenerFactory
	SignalContextFactory SignalContextFactory
}

// NewFoundation synchronously assembles the validated foundation server and its lifecycle owner.
func NewFoundation(options FoundationOptions) (*Application, error) {
	lookupEnv := options.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	serverConfig, err := config.LoadServerYAML(options.ServerYAML, lookupEnv)
	if err != nil {
		return nil, fmt.Errorf("load server configuration: %w", err)
	}

	logWriter := options.LogWriter
	if logWriter == nil {
		logWriter = os.Stderr
	}
	logger := newFoundationLogger(serverConfig, logWriter)
	observer := observability.NewObserver()

	listenerFactory := options.ListenerFactory
	if listenerFactory == nil {
		listenerFactory = web.ListenerFactory(net.Listen)
	}
	server := web.NewServer(serverConfig, observer, logger, options.Version, listenerFactory)

	return newApplication(
		serverConfig.ShutdownTimeout,
		observer,
		logger,
		options.SignalContextFactory,
		server,
	), nil
}

func newFoundationLogger(serverConfig config.ServerConfig, writer io.Writer) *slog.Logger {
	if serverConfig.LogFormat == config.LogFormatJSON {
		return observability.NewLogger(writer, slog.LevelInfo)
	}
	return observability.NewConsoleLogger(writer, slog.LevelInfo)
}
