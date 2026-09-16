package provider

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Karlsk/oryxos-go/internal/config"
	"github.com/Karlsk/oryxos-go/internal/profile"
	"github.com/cloudwego/eino/components/model"
)

// Registry owns factory routing by Provider name and model instances by Profile name.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]ModelFactory
	models    map[string]model.ToolCallingChatModel
}

// ProviderRegistry is the explicit architecture name for the Provider registry.
type ProviderRegistry = Registry

// NewRegistry returns an empty Registry for explicit factory registration.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]ModelFactory),
		models:    make(map[string]model.ToolCallingChatModel),
	}
}

// NewProviderRegistry returns an empty ProviderRegistry for explicit factory registration.
func NewProviderRegistry() *ProviderRegistry { return NewRegistry() }

// NewDefaultRegistry registers the two core Provider factories.
func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	for name, factory := range defaultModelFactories(productionConnectorConstructors()) {
		registry.factories[name] = factory
	}
	return registry
}

// RegisterFactory adds one explicit factory without overwriting an existing name.
func (registry *Registry) RegisterFactory(name string, factory ModelFactory) error {
	if registry == nil {
		return fmt.Errorf("register provider factory: registry is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" || factory == nil {
		return fmt.Errorf("register provider factory: name and factory are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.factories[name]; exists {
		return fmt.Errorf("register provider factory %q: duplicate", name)
	}
	registry.factories[name] = factory
	return nil
}

// BindProfile constructs and stores one isolated model under Profile.name.
func (registry *Registry) BindProfile(ctx context.Context, selected *profile.Profile, definition config.ProviderDefinition) error {
	if registry == nil {
		return fmt.Errorf("bind profile: registry is nil")
	}
	if ctx == nil {
		return fmt.Errorf("bind profile: context is nil")
	}
	if selected == nil || strings.TrimSpace(selected.Name) == "" {
		return fmt.Errorf("bind profile: profile and profile name are required")
	}
	if selected.Provider.Name != definition.Name {
		return fmt.Errorf("bind profile %q: provider declaration mismatch", selected.Name)
	}
	if strings.TrimSpace(definition.APIKey) == "" {
		return fmt.Errorf("bind profile %q: provider credential is empty", selected.Name)
	}

	registry.mu.RLock()
	_, alreadyBound := registry.models[selected.Name]
	factory, found := registry.factories[selected.Provider.Name]
	registry.mu.RUnlock()
	if alreadyBound {
		return fmt.Errorf("bind profile %q: duplicate model binding", selected.Name)
	}
	if !found {
		return fmt.Errorf("bind profile %q: provider factory %q not found", selected.Name, selected.Provider.Name)
	}
	configured, err := factory(ctx, ProviderConfig{
		Name:        definition.Name,
		Model:       selected.Provider.Model,
		APIKey:      definition.APIKey,
		Temperature: selected.Provider.Temperature,
	})
	if err != nil {
		return safeWrap(fmt.Sprintf("bind profile %q: construct provider model", selected.Name), err)
	}
	if configured == nil {
		return fmt.Errorf("bind profile %q: factory returned nil model", selected.Name)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.models[selected.Name]; exists {
		return fmt.Errorf("bind profile %q: duplicate model binding", selected.Name)
	}
	registry.models[selected.Name] = configured
	return nil
}

// BindProfiles atomically rebuilds the complete model snapshot from current Profiles
// and process declarations. A failed rebuild leaves the previous snapshot untouched.
func (registry *Registry) BindProfiles(ctx context.Context, profiles *profile.Registry, definitions []config.ProviderDefinition) error {
	if registry == nil {
		return fmt.Errorf("bind profiles: registry is nil")
	}
	if ctx == nil {
		return fmt.Errorf("bind profiles: context is nil")
	}
	if profiles == nil {
		return fmt.Errorf("bind profiles: profile registry is nil")
	}
	byName := make(map[string]config.ProviderDefinition, len(definitions))
	for _, definition := range definitions {
		if strings.TrimSpace(definition.Name) == "" || strings.TrimSpace(definition.APIKey) == "" {
			return fmt.Errorf("bind profiles: provider name and credential are required")
		}
		if _, duplicate := byName[definition.Name]; duplicate {
			return fmt.Errorf("bind profiles: duplicate provider declaration %q", definition.Name)
		}
		byName[definition.Name] = definition
	}

	registry.mu.RLock()
	factories := make(map[string]ModelFactory, len(registry.factories))
	for name, factory := range registry.factories {
		factories[name] = factory
	}
	registry.mu.RUnlock()

	rebuilt := make(map[string]model.ToolCallingChatModel, profiles.Len())
	for _, selected := range profiles.List() {
		definition, ok := byName[selected.Provider.Name]
		if !ok {
			return fmt.Errorf("bind profile %q: provider %q is undeclared", selected.Name, selected.Provider.Name)
		}
		factory, ok := factories[selected.Provider.Name]
		if !ok {
			return fmt.Errorf("bind profile %q: provider factory %q not found", selected.Name, selected.Provider.Name)
		}
		configured, err := factory(ctx, ProviderConfig{
			Name:        definition.Name,
			Model:       selected.Provider.Model,
			APIKey:      definition.APIKey,
			Temperature: selected.Provider.Temperature,
		})
		if err != nil {
			return safeWrap(fmt.Sprintf("bind profile %q: construct provider model", selected.Name), err)
		}
		if configured == nil {
			return fmt.Errorf("bind profile %q: factory returned nil model", selected.Name)
		}
		rebuilt[selected.Name] = configured
	}

	registry.mu.Lock()
	registry.models = rebuilt
	registry.mu.Unlock()
	return nil
}

// Model returns the isolated model bound to Profile name.
func (registry *Registry) Model(profileName string) (model.ToolCallingChatModel, bool) {
	if registry == nil {
		return nil, false
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	configured, ok := registry.models[profileName]
	return configured, ok
}
