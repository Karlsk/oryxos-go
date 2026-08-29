package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListenAddress     = "127.0.0.1:8080"
	defaultLogFormat         = "console"
	defaultReadHeaderTimeout = "5s"
	defaultReadTimeout       = "30s"
	defaultWriteTimeout      = "5m"
	defaultIdleTimeout       = "60s"
	defaultShutdownTimeout   = "30s"
)

type configError struct {
	path    string
	message string
	cause   error
}

func (errorValue *configError) Error() string {
	return fmt.Sprintf("config %s: %s", errorValue.path, SanitizeErrorString(errorValue.message))
}

func (errorValue *configError) Unwrap() error { return errorValue.cause }

func newConfigError(path, message string, cause error) error {
	return &configError{path: path, message: message, cause: cause}
}

// LoadServerYAML expands, strictly decodes, defaults, and validates server YAML.
func LoadServerYAML(data []byte, lookupEnv func(string) (string, bool)) (ServerConfig, error) {
	document, err := decodeSingleDocument(data)
	if err != nil {
		return ServerConfig{}, err
	}
	if document.Kind != 0 {
		if err := rejectDuplicateKeys(&document, nil); err != nil {
			return ServerConfig{}, err
		}
		if err := expandScalars(&document, nil, lookupEnv); err != nil {
			return ServerConfig{}, err
		}
		if err := validateYAMLShape(&document); err != nil {
			return ServerConfig{}, err
		}
	}

	raw, err := decodeStrictRaw(document)
	if err != nil {
		return ServerConfig{}, err
	}
	return validateServerConfig(raw)
}

func decodeSingleDocument(data []byte) (yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
		return yaml.Node{}, newConfigError("document", "invalid YAML document", err)
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return yaml.Node{}, newConfigError("document", "invalid trailing YAML document", err)
	} else if err == nil {
		return yaml.Node{}, newConfigError("document", "must contain exactly one YAML document", nil)
	}
	return document, nil
}

func rejectDuplicateKeys(node *yaml.Node, path []string) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := rejectDuplicateKeys(child, path); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if _, ok := seen[key.Value]; ok {
				return newConfigError(configPath(append(path, key.Value)), "duplicate key", nil)
			}
			seen[key.Value] = struct{}{}
			if err := rejectDuplicateKeys(value, append(path, key.Value)); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := rejectDuplicateKeys(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateYAMLShape(document *yaml.Node) error {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return newConfigError("document", "must be a mapping", nil)
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		key := root.Content[index]
		value := root.Content[index+1]
		switch key.Value {
		case "listen_address", "log_format", "shutdown_timeout":
			if err := validateScalarString(value, key.Value); err != nil {
				return err
			}
		case "http":
			if value.Kind != yaml.MappingNode {
				return newConfigError("http", "must be a mapping", nil)
			}
			for nestedIndex := 0; nestedIndex+1 < len(value.Content); nestedIndex += 2 {
				nestedKey := value.Content[nestedIndex]
				nestedValue := value.Content[nestedIndex+1]
				switch nestedKey.Value {
				case "read_header_timeout", "read_timeout", "write_timeout", "idle_timeout":
					if err := validateScalarString(nestedValue, "http."+nestedKey.Value); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validateScalarString(node *yaml.Node, path string) error {
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return newConfigError(path, "must not be null", nil)
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return newConfigError(path, "must be a scalar string", nil)
	}
	return nil
}

func decodeStrictRaw(document yaml.Node) (rawServerConfig, error) {
	if document.Kind == 0 || len(document.Content) == 0 {
		return rawServerConfig{}, nil
	}
	var encoded bytes.Buffer
	encoder := yaml.NewEncoder(&encoded)
	if err := encoder.Encode(&document); err != nil {
		return rawServerConfig{}, newConfigError("document", "could not encode YAML document", err)
	}
	if err := encoder.Close(); err != nil {
		return rawServerConfig{}, newConfigError("document", "could not encode YAML document", err)
	}

	decoder := yaml.NewDecoder(&encoded)
	decoder.KnownFields(true)
	var raw rawServerConfig
	if err := decoder.Decode(&raw); err != nil {
		return rawServerConfig{}, strictDecodeError(&document, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return rawServerConfig{}, newConfigError("document", "must contain exactly one YAML document", nil)
		}
		return rawServerConfig{}, newConfigError("document", "invalid trailing YAML document", err)
	}
	return raw, nil
}

func strictDecodeError(document *yaml.Node, cause error) error {
	field := unknownField(cause.Error())
	if field == "" {
		path := findYAMLDecodeErrorPath(document, cause)
		if path == "" {
			path = "document"
		}
		return newConfigError(path, "invalid server configuration", cause)
	}
	path, key := findYAMLFieldPath(document, field)
	if path == "" {
		path = "document"
	}
	return newConfigError(path, fmt.Sprintf("unknown field %q", key), cause)
}

func findYAMLDecodeErrorPath(document *yaml.Node, cause error) string {
	var typeError *yaml.TypeError
	if !errors.As(cause, &typeError) {
		return ""
	}
	for _, message := range typeError.Errors {
		line, ok := yamlErrorLine(message)
		if !ok {
			continue
		}
		if path := findYAMLFieldPathByLine(document, line); path != "" {
			return path
		}
	}
	return ""
}

func yamlErrorLine(message string) (int, bool) {
	const prefix = "line "
	if !strings.HasPrefix(message, prefix) {
		return 0, false
	}
	end := strings.IndexByte(message[len(prefix):], ':')
	if end < 0 {
		return 0, false
	}
	line, err := strconv.Atoi(message[len(prefix) : len(prefix)+end])
	return line, err == nil
}

func findYAMLFieldPathByLine(document *yaml.Node, line int) string {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return ""
	}
	return findMappingFieldPathByLine(document.Content[0], nil, line)
}

func findMappingFieldPathByLine(node *yaml.Node, path []string, line int) string {
	if node.Kind != yaml.MappingNode {
		return ""
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		//nolint:gocritic // Copying preserves the caller's field-path slice.
		fieldPath := append(path, key.Value)
		if nestedPath := findMappingFieldPathByLine(value, fieldPath, line); nestedPath != "" {
			return nestedPath
		}
		if value.Line == line || key.Line == line {
			return configPath(fieldPath)
		}
	}
	return ""
}

func unknownField(message string) string {
	const marker = "field "
	start := strings.Index(message, marker)
	if start < 0 {
		return ""
	}
	remainder := message[start+len(marker):]
	end := strings.Index(remainder, " not found")
	if end < 0 {
		return ""
	}
	return remainder[:end]
}

func findYAMLFieldPath(document *yaml.Node, field string) (string, string) {
	if document.Kind != yaml.DocumentNode || len(document.Content) == 0 {
		return "", field
	}
	return findMappingField(document.Content[0], nil, field)
}

func findMappingField(node *yaml.Node, path []string, field string) (string, string) {
	if node.Kind != yaml.MappingNode {
		return "", field
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		key := node.Content[index]
		value := node.Content[index+1]
		if key.Value == field {
			return configPath(path), key.Value
		}
		if foundPath, foundKey := findMappingField(value, append(path, key.Value), field); foundPath != "" {
			return foundPath, foundKey
		}
	}
	return "", field
}

func validateServerConfig(raw rawServerConfig) (ServerConfig, error) {
	listenAddress := stringOrDefault(raw.ListenAddress, defaultListenAddress)
	if err := validateListenAddress(listenAddress); err != nil {
		return ServerConfig{}, err
	}

	logFormat := stringOrDefault(raw.LogFormat, defaultLogFormat)
	if logFormat != string(LogFormatConsole) && logFormat != string(LogFormatJSON) {
		return ServerConfig{}, newConfigError("log_format", `must be one of "console" or "json"`, nil)
	}

	readHeaderTimeout, err := parsePositiveDuration("http.read_header_timeout", stringOrDefault(rawHTTPValue(raw.HTTP, func(http *rawHTTP) *string { return http.ReadHeaderTimeout }), defaultReadHeaderTimeout))
	if err != nil {
		return ServerConfig{}, err
	}
	readTimeout, err := parsePositiveDuration("http.read_timeout", stringOrDefault(rawHTTPValue(raw.HTTP, func(http *rawHTTP) *string { return http.ReadTimeout }), defaultReadTimeout))
	if err != nil {
		return ServerConfig{}, err
	}
	writeTimeout, err := parsePositiveDuration("http.write_timeout", stringOrDefault(rawHTTPValue(raw.HTTP, func(http *rawHTTP) *string { return http.WriteTimeout }), defaultWriteTimeout))
	if err != nil {
		return ServerConfig{}, err
	}
	idleTimeout, err := parsePositiveDuration("http.idle_timeout", stringOrDefault(rawHTTPValue(raw.HTTP, func(http *rawHTTP) *string { return http.IdleTimeout }), defaultIdleTimeout))
	if err != nil {
		return ServerConfig{}, err
	}
	shutdownTimeout, err := parsePositiveDuration("shutdown_timeout", stringOrDefault(raw.ShutdownTimeout, defaultShutdownTimeout))
	if err != nil {
		return ServerConfig{}, err
	}

	return ServerConfig{
		ListenAddress:     listenAddress,
		LogFormat:         LogFormat(logFormat),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
}

func rawHTTPValue(http *rawHTTP, field func(*rawHTTP) *string) *string {
	if http == nil {
		return nil
	}
	return field(http)
}

func stringOrDefault(value *string, defaultValue string) string {
	if value == nil {
		return defaultValue
	}
	return *value
}

func validateListenAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return newConfigError("listen_address", "must be a non-empty host:port with a port from 1 through 65535", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return newConfigError("listen_address", "must be a non-empty host:port with a port from 1 through 65535", err)
	}
	return nil
}

func parsePositiveDuration(path, value string) (time.Duration, error) {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, newConfigError(path, "must be a Go duration greater than zero", err)
	}
	return duration, nil
}
