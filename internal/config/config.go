// Package config loads validated process-level server configuration.
package config

import "time"

// ServerConfig contains the process-level settings used to run the HTTP server.
type ServerConfig struct {
	ListenAddress     string
	LogFormat         LogFormat
	Providers         []ProviderDefinition
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// ProviderDefinition declares one explicitly supported Provider and its credential.
type ProviderDefinition struct {
	Name   string
	APIKey string
}

// LogFormat selects the application logger's output mode.
type LogFormat string

// Supported log formats.
const (
	LogFormatConsole LogFormat = "console"
	LogFormatJSON    LogFormat = "json"
)

type rawServerConfig struct {
	ListenAddress   *string                 `yaml:"listen_address"`
	LogFormat       *string                 `yaml:"log_format"`
	Providers       []rawProviderDefinition `yaml:"providers"`
	HTTP            *rawHTTP                `yaml:"http"`
	ShutdownTimeout *string                 `yaml:"shutdown_timeout"`
}

type rawProviderDefinition struct {
	Name   *string `yaml:"name"`
	APIKey *string `yaml:"api_key"`
}

type rawHTTP struct {
	ReadHeaderTimeout *string `yaml:"read_header_timeout"`
	ReadTimeout       *string `yaml:"read_timeout"`
	WriteTimeout      *string `yaml:"write_timeout"`
	IdleTimeout       *string `yaml:"idle_timeout"`
}
