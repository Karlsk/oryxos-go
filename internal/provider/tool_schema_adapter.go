package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// ToolInfoSource exposes metadata only; it cannot execute a Tool.
type ToolInfoSource interface {
	Info(ctx context.Context, name string) (*schema.ToolInfo, bool)
}

// ToolSchemaAdapter translates Profile Tool names into Eino metadata.
type ToolSchemaAdapter interface {
	ToEinoToolInfos(ctx context.Context, names []string) ([]*schema.ToolInfo, error)
}

type metadataToolSchemaAdapter struct{ source ToolInfoSource }

// NewToolSchemaAdapter constructs a metadata-only adapter.
func NewToolSchemaAdapter(source ToolInfoSource) (ToolSchemaAdapter, error) {
	if source == nil {
		return nil, fmt.Errorf("create tool schema adapter: source is nil")
	}
	return &metadataToolSchemaAdapter{source: source}, nil
}

func (adapter *metadataToolSchemaAdapter) ToEinoToolInfos(ctx context.Context, names []string) ([]*schema.ToolInfo, error) {
	if ctx == nil {
		return nil, fmt.Errorf("convert tool schemas: context is nil")
	}
	if len(names) == 0 {
		return nil, nil
	}
	infos := make([]*schema.ToolInfo, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("convert tool schemas: tool name is empty")
		}
		info, ok := adapter.source.Info(ctx, name)
		if !ok || info == nil {
			return nil, fmt.Errorf("convert tool schemas: tool %q not found", name)
		}
		infos = append(infos, info)
	}
	return infos, nil
}
