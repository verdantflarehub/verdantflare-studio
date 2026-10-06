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
