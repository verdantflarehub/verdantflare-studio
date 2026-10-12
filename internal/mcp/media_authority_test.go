package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestVerifiedUserCanCallMediaWithoutSeparateApproval(t *testing.T) {
	// Stale deployment variables must not restore the removed approval gate.
	t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
	t.Setenv("STUDIO_GENERATION_APPROVAL_TOKEN", "unused-legacy-value")
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	pid := id()
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID || r.Header.Get("X-Request-Id") != p.RequestID || r.Header.Get("X-Project-Id") != pid {
			t.Error("verified caller identity was not forwarded")
		}
		if r.Header.Get("Authorization") != "Bearer internal-media-token" {
			t.Error("incorrect downstream credential")
		}
		var request struct {
			Params struct {
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil || request.Params.Arguments["project_id"] != pid || request.Params.Arguments["idempotency_key"] != "existing-operation" {
			t.Error("media request changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": p.RequestID, "result": map[string]any{"isError": false}})
	}))
	defer provider.Close()
	g := NewGatewayWithClient(nil)
	for _, domain := range []string{"image", "video", "music"} {
		t.Setenv("STUDIO_"+strings.ToUpper(domain)+"_TOKEN", "internal-media-token")
		names := []string{domain + ".create"}
		if domain == "video" {
			names = append(names, "video.import", "video.import_prepare", "video.import_chunk", "video.import_status", "video.import_commit", "video.preflight")
		}
		reg := ServiceRegistration{Domain: domain, Endpoint: provider.URL}
		for _, name := range names {
			reg.Tools = append(reg.Tools, ToolDefinition{Name: name})
		}
		g.services[domain] = reg
		for _, name := range names {
			args := map[string]any{"project_id": pid, "idempotency_key": "existing-operation"}
			before := calls
			if _, status, err := g.CallVerified(context.Background(), name, args, p, pid); status != 200 || err != nil || calls != before+1 {
				t.Fatalf("verified media call blocked: %s %d %v", name, status, err)
			}
			if _, status, _ := g.CallVerified(context.Background(), name, args, project.Principal{}, pid); status != 403 || calls != before+1 {
				t.Fatal("invalid identity reached media service")
			}
		}
	}
	for _, tool := range project.MCPTools() {
		if strings.HasPrefix(tool.Name, "project.generation_") {
			t.Fatal("removed approval workflow still advertised")
		}
	}
}
