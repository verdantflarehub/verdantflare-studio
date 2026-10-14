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
