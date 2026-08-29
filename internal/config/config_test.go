package config

import (
	"strings"
	"testing"
	"time"
)

func defaultServerConfig() ServerConfig {
	return ServerConfig{
		ListenAddress:     "127.0.0.1:8080",
		LogFormat:         LogFormatConsole,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
		ShutdownTimeout:   30 * time.Second,
	}
}

func unsetLookup(string) (string, bool) { return "", false }

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("LoadServerYAML() error = %v", err)
	}
}

func TestLoadServerYAMLDefaults(t *testing.T) {
	got, err := LoadServerYAML(nil, unsetLookup)
	requireNoError(t, err)
	if got != defaultServerConfig() {
		t.Fatalf("LoadServerYAML() = %#v, want %#v", got, defaultServerConfig())
	}
	if got.ReadHeaderTimeout == 0 || got.ReadTimeout == 0 || got.WriteTimeout == 0 || got.IdleTimeout == 0 || got.ShutdownTimeout == 0 {
		t.Fatal("LoadServerYAML() returned a zero timeout")
	}
}

func TestLoadServerYAMLPartialDefaults(t *testing.T) {
	got, err := LoadServerYAML([]byte("listen_address: 127.0.0.1:9090\n"), unsetLookup)
	requireNoError(t, err)
	want := defaultServerConfig()
	want.ListenAddress = "127.0.0.1:9090"
	if got != want {
		t.Fatalf("LoadServerYAML() = %#v, want %#v", got, want)
	}
}

func TestLoadServerYAMLExpansion(t *testing.T) {
	got, err := LoadServerYAML([]byte("listen_address: ${ORYXOS_LISTEN_ADDRESS}\n"), func(name string) (string, bool) {
		if name == "ORYXOS_LISTEN_ADDRESS" {
			return "127.0.0.1:9090", true
		}
		return "", false
	})
	requireNoError(t, err)
	if got.ListenAddress != "127.0.0.1:9090" {
		t.Fatalf("ListenAddress = %q, want %q", got.ListenAddress, "127.0.0.1:9090")
	}
}

func TestLoadServerYAMLMissingVariable(t *testing.T) {
	const yaml = "listen_address: ${ORYXOS_LISTEN_ADDRESS}\n"
	_, err := LoadServerYAML([]byte(yaml), func(string) (string, bool) {
		return "unrelated-environment-value", false
	})
	if err == nil {
		t.Fatal("LoadServerYAML() error = nil, want missing variable error")
	}
	message := err.Error()
	for _, want := range []string{"config listen_address:", "ORYXOS_LISTEN_ADDRESS"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
	for _, forbidden := range []string{yaml, "unrelated-environment-value", "${"} {
		if strings.Contains(message, forbidden) {
			t.Errorf("error %q contains %q", message, forbidden)
		}
	}
}

func TestLoadServerYAMLInvalidDuration(t *testing.T) {
	_, err := LoadServerYAML([]byte("http:\n  write_timeout: nope\n"), unsetLookup)
	if err == nil {
		t.Fatal("LoadServerYAML() error = nil, want invalid duration error")
	}
	if !strings.Contains(err.Error(), "config http.write_timeout:") {
		t.Fatalf("error = %q, want http.write_timeout path", err)
	}
}

func TestLoadServerYAMLZeroOrNegativeTimeouts(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		path string
	}{
		{"read_header_timeout_0s", "http:\n  read_header_timeout: 0s\n", "http.read_header_timeout"},
		{"read_header_timeout_negative", "http:\n  read_header_timeout: -1s\n", "http.read_header_timeout"},
		{"read_timeout_0s", "http:\n  read_timeout: 0s\n", "http.read_timeout"},
		{"read_timeout_negative", "http:\n  read_timeout: -1s\n", "http.read_timeout"},
		{"write_timeout_0s", "http:\n  write_timeout: 0s\n", "http.write_timeout"},
		{"write_timeout_negative", "http:\n  write_timeout: -1s\n", "http.write_timeout"},
		{"idle_timeout_0s", "http:\n  idle_timeout: 0s\n", "http.idle_timeout"},
		{"idle_timeout_negative", "http:\n  idle_timeout: -1s\n", "http.idle_timeout"},
		{"shutdown_timeout_0s", "shutdown_timeout: 0s\n", "shutdown_timeout"},
		{"shutdown_timeout_negative", "shutdown_timeout: -1s\n", "shutdown_timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadServerYAML([]byte(tc.yaml), unsetLookup)
			if err == nil {
				t.Fatal("LoadServerYAML() error = nil, want positive duration error")
			}
			if !strings.Contains(err.Error(), "config "+tc.path+":") || !strings.Contains(err.Error(), "greater than zero") {
				t.Fatalf("error = %q, want positive duration error at %s", err, tc.path)
			}
		})
	}
}

func TestLoadServerYAMLStrictUnknownFields(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		contains  []string
		forbidden []string
	}{
		{"nested_typo", "http:\n  write_timout: 5m\n", []string{"config http:", "write_timout"}, nil},
		{"profile_key", "provider: {}\n", []string{"provider"}, nil},
		{"duplicate_key", "log_format: console\nlog_format: json\n", []string{"log_format"}, nil},
		{"trailing_document", "log_format: console\n---\nlog_format: json\n", []string{"config"}, nil},
		{"malformed_placeholder", "listen_address: ${9INVALID}\n", []string{"config listen_address:", "malformed"}, nil},
		{"empty_expansion", "listen_address: ${ORYXOS_LISTEN_ADDRESS}\n", []string{"config listen_address:"}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := unsetLookup
			if tc.name == "empty_expansion" {
				lookup = func(name string) (string, bool) { return "", name == "ORYXOS_LISTEN_ADDRESS" }
			}
			_, err := LoadServerYAML([]byte(tc.yaml), lookup)
			if err == nil {
				t.Fatal("LoadServerYAML() error = nil, want strict decoding or validation error")
			}
			for _, want := range tc.contains {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			for _, forbidden := range tc.forbidden {
				if strings.Contains(err.Error(), forbidden) {
					t.Errorf("error %q contains %q", err, forbidden)
				}
			}
		})
	}
}

func TestLoadServerYAMLInvalidAddress(t *testing.T) {
	_, err := LoadServerYAML([]byte("listen_address: localhost\n"), unsetLookup)
	if err == nil {
		t.Fatal("LoadServerYAML() error = nil, want invalid address error")
	}
	if !strings.Contains(err.Error(), "config listen_address:") {
		t.Fatalf("error = %q, want listen_address path", err)
	}
}

func TestLoadServerYAMLLogFormat(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		want    LogFormat
		wantErr bool
	}{
		{"json", "log_format: json\n", LogFormatJSON, false},
		{"xml", "log_format: xml\n", "", true},
		{"uppercase_json", "log_format: JSON\n", "", true},
		{"omitted", "", LogFormatConsole, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LoadServerYAML([]byte(tc.yaml), unsetLookup)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "config log_format:") || !strings.Contains(err.Error(), "console") || !strings.Contains(err.Error(), "json") {
					t.Fatalf("error = %v, want invalid log_format error", err)
				}
				return
			}
			requireNoError(t, err)
			if got.LogFormat != tc.want {
				t.Fatalf("LogFormat = %q, want %q", got.LogFormat, tc.want)
			}
		})
	}
}

func TestLoadServerYAMLRejectsNullAndUnexpectedScalarTypes(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		path string
	}{
		{"null_listen_address", "listen_address: null\n", "listen_address"},
		{"null_log_format", "log_format: null\n", "log_format"},
		{"null_shutdown_timeout", "shutdown_timeout: null\n", "shutdown_timeout"},
		{"null_read_header_timeout", "http:\n  read_header_timeout: null\n", "http.read_header_timeout"},
		{"null_read_timeout", "http:\n  read_timeout: null\n", "http.read_timeout"},
		{"null_write_timeout", "http:\n  write_timeout: null\n", "http.write_timeout"},
		{"null_idle_timeout", "http:\n  idle_timeout: null\n", "http.idle_timeout"},
		{"sequence_listen_address", "listen_address: [very-secret-sentinel]\n", "listen_address"},
		{"mapping_log_format", "log_format: {value: very-secret-sentinel}\n", "log_format"},
		{"sequence_shutdown_timeout", "shutdown_timeout: [very-secret-sentinel]\n", "shutdown_timeout"},
		{"mapping_read_header_timeout", "http:\n  read_header_timeout: {value: very-secret-sentinel}\n", "http.read_header_timeout"},
		{"sequence_read_timeout", "http:\n  read_timeout: [very-secret-sentinel]\n", "http.read_timeout"},
		{"mapping_write_timeout", "http:\n  write_timeout: {value: very-secret-sentinel}\n", "http.write_timeout"},
		{"sequence_idle_timeout", "http:\n  idle_timeout: [very-secret-sentinel]\n", "http.idle_timeout"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadServerYAML([]byte(tc.yaml), unsetLookup)
			if err == nil {
				t.Fatal("LoadServerYAML() error = nil, want scalar null or type error")
			}
			if !strings.Contains(err.Error(), "config "+tc.path+":") {
				t.Errorf("error = %q, want stable path %q", err, tc.path)
			}
			for _, forbidden := range []string{"config document:", "very-secret-sentinel", tc.yaml} {
				if strings.Contains(err.Error(), forbidden) {
					t.Errorf("error %q contains %q", err, forbidden)
				}
			}
		})
	}
}

func TestLoadServerYAMLRedactsSecrets(t *testing.T) {
	const secretURL = "https://example.invalid/hook/very-secret-token"
	_, err := LoadServerYAML([]byte("webhook_url: "+secretURL+"\n"), unsetLookup)
	if err == nil {
		t.Fatal("LoadServerYAML() error = nil, want strict unknown field error")
	}
	for _, forbidden := range []string{"very-secret-token", secretURL} {
		if strings.Contains(err.Error(), forbidden) {
			t.Errorf("error %q contains secret %q", err, forbidden)
		}
	}
	if got := SanitizeErrorString("api_key: top-secret"); got != "[REDACTED]" {
		t.Fatalf("SanitizeErrorString() = %q, want %q", got, "[REDACTED]")
	}
}
