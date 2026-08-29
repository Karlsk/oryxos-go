// Package config loads validated process-level server configuration.
package config

import "time"

// ServerConfig contains the process-level settings used to run the HTTP server.
type ServerConfig struct {
	ListenAddress     string
	LogFormat         LogFormat
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// LogFormat selects the application logger's output mode.
type LogFormat string

// Supported log formats.
const (
	LogFormatConsole LogFormat = "console"
	LogFormatJSON    LogFormat = "json"
)

type rawServerConfig struct {
	ListenAddress   *string  `yaml:"listen_address"`
	LogFormat       *string  `yaml:"log_format"`
	HTTP            *rawHTTP `yaml:"http"`
	ShutdownTimeout *string  `yaml:"shutdown_timeout"`
}

type rawHTTP struct {
	ReadHeaderTimeout *string `yaml:"read_header_timeout"`
	ReadTimeout       *string `yaml:"read_timeout"`
	WriteTimeout      *string `yaml:"write_timeout"`
	IdleTimeout       *string `yaml:"idle_timeout"`
}
