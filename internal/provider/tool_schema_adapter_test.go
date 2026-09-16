package provider

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestToolSchemaAdapterPreservesOrderAndTruthfulMetadata(t *testing.T) {
	read := &schema.ToolInfo{Name: "read_file", Desc: "read a file"}
	write := &schema.ToolInfo{Name: "write_file", Desc: "write a file"}
	source := &fakeToolInfoSource{infos: map[string]*schema.ToolInfo{"read_file": read, "write_file": write}}
	adapter, err := NewToolSchemaAdapter(source)
	if err != nil {
		t.Fatalf("NewToolSchemaAdapter() error = %v", err)
	}
	got, err := adapter.ToEinoToolInfos(context.Background(), []string{"write_file", "read_file"})
	if err != nil {
		t.Fatalf("ToEinoToolInfos() error = %v", err)
	}
	if len(got) != 2 || got[0] != write || got[1] != read {
		t.Fatalf("Tool infos = %#v, want source metadata in requested order", got)
	}
	if len(source.calls) != 2 || source.calls[0] != "write_file" || source.calls[1] != "read_file" {
		t.Fatalf("source calls = %v, want ordered lookups", source.calls)
	}
}

func TestToolSchemaAdapterRejectsUnknownAndInvalidInputs(t *testing.T) {
	if _, err := NewToolSchemaAdapter(nil); err == nil {
		t.Fatal("NewToolSchemaAdapter(nil) error = nil")
	}
	source := &fakeToolInfoSource{infos: map[string]*schema.ToolInfo{"nil_info": nil}}
	adapter, _ := NewToolSchemaAdapter(source)
	for _, names := range [][]string{{"missing"}, {""}, {"nil_info"}} {
		if _, err := adapter.ToEinoToolInfos(context.Background(), names); err == nil {
			t.Fatalf("ToEinoToolInfos(%v) error = nil", names)
		}
	}
	got, err := adapter.ToEinoToolInfos(context.Background(), nil)
	if err != nil || got != nil {
		t.Fatalf("ToEinoToolInfos(nil) = %#v, %v; want nil, nil", got, err)
	}
}
