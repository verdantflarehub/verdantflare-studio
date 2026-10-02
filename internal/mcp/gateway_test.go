package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGateway_ListToolsAndCall(t *testing.T) {
	// Create mock downstream video microservice
	videoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-Id") != "usr_123" {
			t.Errorf("expected X-User-Id usr_123, got %s", r.Header.Get("X-User-Id"))
		}
		if r.Header.Get("X-Project-Id") != "prj_456" {
			t.Errorf("expected X-Project-Id prj_456, got %s", r.Header.Get("X-Project-Id"))
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"jsonrpc": "2.0",
			"id":      body["id"],
			"result": map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "Video task created"},
				},
				"data": map[string]any{
					"task_id": "tsk_test_001",
					"status":  "pending",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer videoServer.Close()

	// Initialize Gateway directly
	gw := NewGatewayWithClient(nil)
	gw.services["video"] = ServiceRegistration{
		Domain:         "video",
		Endpoint:       videoServer.URL,
		HealthEndpoint: videoServer.URL + "/health",
		Version:        "v0.1.20",
		Tools: []ToolDefinition{
			{
				Name:        "video.create",
				Description: "Create a video",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}
	gw.services["image"] = ServiceRegistration{
		Domain:         "image",
		Endpoint:       "http://image-service:8000/mcp",
		HealthEndpoint: "http://image-service:8000/health",
		Version:        "v0.1.0",
		Tools: []ToolDefinition{
			{
				Name:        "image.create",
				Description: "Generate an image",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	// 1. Test ListTools
	tools := gw.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 aggregated tools, got %d", len(tools))
	}

	// 2. Test CallTool routing
	headers := map[string]string{
		"X-User-Id":    "usr_123",
		"X-Project-Id": "prj_456",
		"X-Request-Id": "req_789",
	}
	result, status, err := gw.CallTool(context.Background(), "video.create", map[string]any{"prompt": "neon city"}, headers)
	if err != nil {
		t.Fatalf("unexpected error calling tool: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	data, ok := resMap["data"].(map[string]any)
	if !ok || data["task_id"] != "tsk_test_001" {
		t.Fatalf("unexpected result data: %v", resMap)
	}

	// 3. Test Unknown Domain Tool Call
	_, status, err = gw.CallTool(context.Background(), "music.create", nil, headers)
	if err == nil || status != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown domain, got status %d, err %v", status, err)
	}
}
