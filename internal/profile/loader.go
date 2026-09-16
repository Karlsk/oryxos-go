package profile

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/config"
	"gopkg.in/yaml.v3"
)

const defaultTemperature float32 = 0.7

var environmentPlaceholder = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
var unknownYAMLField = regexp.MustCompile(`field ([^ ]+) not found`)

// Loader reads a directory into one immutable Registry snapshot.
type Loader struct {
	LookupEnv func(string) (string, bool)
}

// ProfileLoader is the explicit architecture name for the Profile loader.
type ProfileLoader = Loader

// Diagnostic describes one Profile file that was skipped without exposing secrets.
type Diagnostic struct {
	Path string
	Err  error
}

func (diagnostic Diagnostic) Error() string {
	message := "invalid profile"
	if diagnostic.Err != nil {
		message = config.SanitizeErrorString(diagnostic.Err.Error())
	}
	return fmt.Sprintf("profile %s: %s", filepath.Base(diagnostic.Path), message)
}

// LoadDirectory strictly loads YAML files in lexical order. Per-file errors become
// diagnostics; directory-level failures are returned.
func (loader Loader) LoadDirectory(directory string, declaredProviders map[string]struct{}) (*Registry, []Diagnostic, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, nil, fmt.Errorf("read profile directory: %w", err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension == ".yaml" || extension == ".yml" {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)

	profiles := make(map[string]*Profile, len(paths))
	names := make([]string, 0, len(paths))
	diagnostics := make([]Diagnostic, 0)
	for _, path := range paths {
		loaded, loadErr := loader.loadFile(path, declaredProviders)
		if loadErr != nil {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Err: loadErr})
			continue
		}
		if _, duplicate := profiles[loaded.Name]; duplicate {
			diagnostics = append(diagnostics, Diagnostic{Path: path, Err: fmt.Errorf("duplicate profile name %q", loaded.Name)})
			continue
		}
		profiles[loaded.Name] = loaded
		names = append(names, loaded.Name)
	}
	return newRegistry(profiles, names), diagnostics, nil
}

func (loader Loader) loadFile(path string, declaredProviders map[string]struct{}) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	expanded, err := loader.expandEnvironment(data)
	if err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(expanded))
	decoder.KnownFields(true)
	var raw rawProfile
	if err := decoder.Decode(&raw); err != nil {
		if match := unknownYAMLField.FindStringSubmatch(err.Error()); len(match) == 2 {
			return nil, fmt.Errorf("unknown field %q", match[1])
		}
		return nil, fmt.Errorf("strict YAML decode: %s", config.SanitizeErrorString(err.Error()))
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("must contain exactly one YAML document")
		}
		return nil, fmt.Errorf("decode trailing YAML: %w", err)
	}
	return validateProfile(raw, declaredProviders)
}

func (loader Loader) expandEnvironment(data []byte) ([]byte, error) {
	lookup := loader.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}
	var expansionErr error
	expanded := environmentPlaceholder.ReplaceAllStringFunc(string(data), func(placeholder string) string {
		if expansionErr != nil {
			return placeholder
		}
		name := environmentPlaceholder.FindStringSubmatch(placeholder)[1]
		value, ok := lookup(name)
		if !ok {
			expansionErr = fmt.Errorf("environment variable %s is not set", name)
			return placeholder
		}
		return value
	})
	if expansionErr != nil {
		return nil, expansionErr
	}
	return []byte(expanded), nil
}

func validateProfile(raw rawProfile, declaredProviders map[string]struct{}) (*Profile, error) {
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		return nil, fmt.Errorf("name must not be empty")
	}
	providerName := strings.TrimSpace(raw.Provider.Name)
	if providerName == "" {
		return nil, fmt.Errorf("provider.name must not be empty")
	}
	if _, declared := declaredProviders[providerName]; !declared {
		return nil, fmt.Errorf("provider.name %q is undeclared", providerName)
	}
	modelName := strings.TrimSpace(raw.Provider.Model)
	if modelName == "" {
		return nil, fmt.Errorf("provider.model must not be empty")
	}
	temperature := defaultTemperature
	if raw.Provider.Temperature != nil {
		temperature = *raw.Provider.Temperature
	}
	if math.IsNaN(float64(temperature)) || math.IsInf(float64(temperature), 0) {
		return nil, fmt.Errorf("provider.temperature must be finite")
	}

	return &Profile{
		Name:           name,
		Description:    raw.Description,
		Identity:       raw.Identity,
		Provider:       ProviderConfig{Name: providerName, Model: modelName, Temperature: temperature},
		Tools:          append([]string(nil), raw.Tools...),
		Skills:         append([]string(nil), raw.Skills...),
		MCPServers:     append([]string(nil), raw.MCPServers...),
		NotifyChannels: append([]NotifyChannelConfig(nil), raw.NotifyChannels...),
		Schedules:      append([]ScheduleConfig(nil), raw.Schedules...),
		Channels:       cloneChannels(raw.Channels),
		Bootstrap:      append([]string(nil), raw.Bootstrap...),
		Settings:       raw.Settings,
	}, nil
}

func cloneChannels(channels []ChannelConfig) []ChannelConfig {
	cloned := make([]ChannelConfig, len(channels))
	for index, channel := range channels {
		cloned[index] = channel
		if channel.Config != nil {
			cloned[index].Config = make(map[string]any, len(channel.Config))
			for key, value := range channel.Config {
				cloned[index].Config[key] = cloneConfigValue(value)
			}
		}
	}
	return cloned
}

func cloneConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, nested := range typed {
			cloned[key] = cloneConfigValue(nested)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, nested := range typed {
			cloned[index] = cloneConfigValue(nested)
		}
		return cloned
	default:
		return value
	}
}
