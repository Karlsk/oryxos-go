package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/Karlsk/oryxos-go/internal/llm"
)

// ToolDefinitionSource exposes metadata only; it cannot execute a Tool.
type ToolDefinitionSource interface {
	Info(ctx context.Context, name string) (llm.ToolDefinition, bool)
}

// ToolSchemaResolver resolves Profile Tool names into OryxOS metadata.
type ToolSchemaResolver interface {
	Resolve(ctx context.Context, names []string) ([]llm.ToolDefinition, error)
}

type metadataToolSchemaResolver struct{ source ToolDefinitionSource }

// NewToolSchemaResolver constructs a metadata-only resolver.
func NewToolSchemaResolver(source ToolDefinitionSource) (ToolSchemaResolver, error) {
	if source == nil {
		return nil, fmt.Errorf("create tool schema resolver: source is nil")
	}
	return &metadataToolSchemaResolver{source: source}, nil
}

func (resolver *metadataToolSchemaResolver) Resolve(ctx context.Context, names []string) ([]llm.ToolDefinition, error) {
	if ctx == nil {
		return nil, fmt.Errorf("resolve tool schemas: context is nil")
	}
	if len(names) == 0 {
		return nil, nil
	}
	definitions := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("resolve tool schemas: tool name is empty")
		}
		definition, ok := resolver.source.Info(ctx, name)
		if !ok || strings.TrimSpace(definition.Name) == "" {
			return nil, fmt.Errorf("resolve tool schemas: tool %q not found", name)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}
