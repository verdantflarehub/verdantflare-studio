package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestSpeechRequiresVerifiedIdentityAndExplicitProject(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	pid, calls := id(), 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer speech-internal" || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Fatal("incorrect internal principal")
		}
		var in struct {
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		wantProject := pid
		if in.Params.Name == "speech.capabilities" {
			wantProject = ""
		}
		if r.Header.Get("X-Project-Id") != wantProject {
			t.Error("incorrect project scope")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": p.RequestID, "result": map[string]any{"isError": false}})
	}))
	defer provider.Close()
	g := NewGatewayWithClient(nil)
	g.services["speech"] = ServiceRegistration{Domain: "speech", Endpoint: provider.URL, Tools: []ToolDefinition{{Name: "speech.create"}, {Name: "speech.capabilities"}}}
	t.Setenv("STUDIO_SPEECH_TOKEN", "speech-internal")
	if _, status, _ := g.CallTool(context.Background(), "speech.create", nil, nil); status != 403 {
		t.Fatal("unverified speech accepted")
	}
	for _, args := range []map[string]any{nil, {"project_id": "prj_studio_default"}} {
		if _, status, _ := g.CallVerified(context.Background(), "speech.create", args, p, ""); status != 400 {
			t.Fatal("invalid scope accepted")
		}
	}
	if _, status, _ := g.CallVerified(context.Background(), "speech.create", map[string]any{"project_id": pid}, p, id()); status != 400 {
		t.Fatal("scope mismatch accepted")
	}
	if calls != 0 {
		t.Fatal("invalid calls reached producer")
	}
	for _, name := range []string{"speech.create", "speech.capabilities"} {
		args := map[string]any{}
		if name == "speech.create" {
			args["project_id"] = pid
		}
		if _, status, err := g.CallVerified(context.Background(), name, args, p, ""); status != 200 || err != nil {
			t.Fatalf("valid speech blocked: %d %v", status, err)
		}
	}
	t.Setenv("STUDIO_SPEECH_TOKEN", "")
	t.Setenv("SPEECH_MCP_BEARER_TOKEN", "")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "")
	if _, status, _ := g.CallVerified(context.Background(), "speech.capabilities", nil, p, ""); status != 503 || calls != 2 {
		t.Fatal("missing credential reached producer")
	}
}

func TestSpeechListOptionalProjectMatchesRegisteredContract(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	pid, calls := id(), 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer speech-internal" ||
			r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Error("list lost verified identity or internal credential")
		}
		var in struct {
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatal(err)
		}
		want, _ := in.Params.Arguments["project_id"].(string)
		if in.Params.Name != "speech.list" || r.Header.Get("X-Project-Id") != want {
			t.Error("list changed project scope")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": p.RequestID,
			"result": map[string]any{"isError": false, "structuredContent": map[string]any{"tasks": []any{}}}})
	}))
	defer provider.Close()
	g := NewGatewayWithClient(nil)
	g.services["speech"] = ServiceRegistration{Domain: "speech", Endpoint: provider.URL,
		Tools: []ToolDefinition{{Name: "speech.list"}}}
	t.Setenv("STUDIO_SPEECH_TOKEN", "speech-internal")
	for _, tc := range []struct {
		name   string
		args   map[string]any
		header string
		status int
	}{
		{"no filter", nil, "", 200},
		{"pagination only", map[string]any{"limit": 5}, "", 200},
		{"project filter", map[string]any{"project_id": pid}, "", 200},
		{"matching header", map[string]any{"project_id": pid}, pid, 200},
		{"header alone", nil, pid, 400},
		{"mismatched header", map[string]any{"project_id": pid}, id(), 400},
		{"empty project", map[string]any{"project_id": ""}, "", 400},
		{"null project", map[string]any{"project_id": nil}, "", 400},
		{"wrong type", map[string]any{"project_id": 1}, "", 400},
		{"invalid project", map[string]any{"project_id": "prj_studio_default"}, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			_, status, err := g.CallVerified(context.Background(), "speech.list", tc.args, p, tc.header)
			if status != tc.status || (tc.status == 200 && err != nil) {
				t.Fatalf("got status %d error %v, want %d", status, err, tc.status)
			}
			if tc.status != 200 && calls != before {
				t.Fatal("invalid scope reached speech service")
			}
		})
	}
	before := calls
	if _, status, _ := g.CallVerified(context.Background(), "speech.list", nil, project.Principal{}, ""); status != 403 {
		t.Fatal("unverified list accepted")
	}
	if _, status, _ := g.CallTool(context.Background(), "speech.list", nil, nil); status != 403 || calls != before {
		t.Fatal("unverified call reached speech service")
	}
}
