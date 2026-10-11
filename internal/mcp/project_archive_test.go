package mcp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestArchiveDiscoveryPreservesRegistrationAndSchemas(t *testing.T) {
	g := NewGatewayWithClient(nil)
	schema := map[string]any{"type": "object", "properties": map[string]any{"project_id": map[string]any{"type": "string"}}}
	domains := []string{"project", "artifact", "world", "image", "video", "music", "blender", "health"}
	for _, d := range domains {
		g.services[d] = ServiceRegistration{Domain: d, Tools: []ToolDefinition{{Name: d + ".example", Description: "Original", InputSchema: schema}}}
	}
	before, _ := json.Marshal(g.services)
	first := g.ListTools()
	second := g.ListTools()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repeated discovery accumulates guidance")
	}
	for _, tool := range first {
		domain := strings.SplitN(tool.Name, ".", 2)[0]
		if !reflect.DeepEqual(tool.InputSchema, schema) {
			t.Fatalf("changed input schema for %s", tool.Name)
		}
		if (strings.Contains(tool.Description, projectArchiveToolHint)) != (domain != "health") {
			t.Fatalf("wrong guidance scope for %s", tool.Name)
		}
	}
	after, _ := json.Marshal(g.services)
	if string(before) != string(after) {
		t.Fatal("discovery changed registered service definitions")
	}
}
