package provider

import (
	"context"
	"testing"

	"github.com/Karlsk/oryxos-go/internal/llm"
)

func TestToolSchemaResolverPreservesOrderAndTruthfulMetadata(t *testing.T) {
	read := llm.ToolDefinition{Name: "read_file", Description: "read a file"}
	write := llm.ToolDefinition{Name: "write_file", Description: "write a file"}
	source := &fakeToolDefinitionSource{definitions: map[string]llm.ToolDefinition{"read_file": read, "write_file": write}}
	resolver, err := NewToolSchemaResolver(source)
	if err != nil {
		t.Fatalf("NewToolSchemaResolver() error = %v", err)
	}
	got, err := resolver.Resolve(context.Background(), []string{"write_file", "read_file"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(got) != 2 || got[0].Name != write.Name || got[1].Name != read.Name {
		t.Fatalf("Tool definitions = %#v, want source metadata in requested order", got)
	}
	if len(source.calls) != 2 || source.calls[0] != "write_file" || source.calls[1] != "read_file" {
		t.Fatalf("source calls = %v, want ordered lookups", source.calls)
	}
}

func TestToolSchemaResolverRejectsUnknownAndInvalidInputs(t *testing.T) {
	if _, err := NewToolSchemaResolver(nil); err == nil {
		t.Fatal("NewToolSchemaResolver(nil) error = nil")
	}
	source := &fakeToolDefinitionSource{definitions: map[string]llm.ToolDefinition{"empty_definition": {}}}
	resolver, _ := NewToolSchemaResolver(source)
	for _, names := range [][]string{{"missing"}, {""}, {"empty_definition"}} {
		if _, err := resolver.Resolve(context.Background(), names); err == nil {
			t.Fatalf("Resolve(%v) error = nil", names)
		}
	}
	got, err := resolver.Resolve(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("Resolve(nil) = %#v, %v; want nil, nil", got, err)
	}
}
