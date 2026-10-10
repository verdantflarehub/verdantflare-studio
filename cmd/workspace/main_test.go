package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCallUsesUserSessionAndRejectsDuplicateArguments(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.Header.Get("Authorization") != "Bearer private-session" {
			t.Error("missing session")
		}
		var request struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"isError": false, "structuredContent": map[string]any{"items": []any{}, "next_cursor": ""}}})
	}))
	defer server.Close()
	t.Setenv("STUDIO_MCP_URL", server.URL)
	t.Setenv("STUDIO_MCP_BEARER_TOKEN", "private-session")
	var output bytes.Buffer
	if err := run(t.Context(), []string{"call", "--input", "-"}, strings.NewReader(`{"name":"project.list","arguments":{}}`), &output); err != nil {
		t.Fatal(err)
	}
	if hits != 1 || strings.Contains(output.String(), "private-session") || !json.Valid(output.Bytes()) {
		t.Fatal("invalid output or credential exposure")
	}
	if err := run(t.Context(), []string{"call", "--input", "-"}, strings.NewReader(`{"name":"project.list","arguments":{"limit":1,"limit":2}}`), &output); err == nil || hits != 1 {
		t.Fatal("duplicate JSON reached server")
	}
}

func TestToolsUsesOnlyConfiguredBearer(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "all-project-tools", false: "missing-project-tools"}[available], func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/mcp" || r.Header.Get("Authorization") != "Bearer user-token" || r.Header.Get("Cookie") != "" {
					t.Error("discovery tried another authentication path")
				}
				var req struct {
					ID     string `json:"id"`
					Method string `json:"method"`
				}
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.Method != "tools/list" {
					t.Error("unexpected write")
				}
				tools := []map[string]string{{"name": "image.create"}}
				if available {
					for _, name := range []string{"project.create", "project.list", "project.open"} {
						tools = append(tools, map[string]string{"name": name})
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"tools": tools}})
			}))
			defer server.Close()
			t.Setenv("STUDIO_MCP_URL", server.URL)
			t.Setenv("STUDIO_MCP_BEARER_TOKEN", "user-token")
			var output bytes.Buffer
			err := run(t.Context(), []string{"tools"}, strings.NewReader(""), &output)
			if calls != 1 || (available && (err != nil || !strings.Contains(output.String(), "project.create"))) || (!available && (err == nil || !strings.Contains(err.Error(), "server-side Bearer identity"))) {
				t.Fatal("unexpected discovery result", err, calls)
			}
		})
	}
}
