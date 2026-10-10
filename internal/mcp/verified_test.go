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

func TestManagedRoutingRequiresVerifiedIdentityAndExactScope(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	projectID := id()
	calls := 0
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Error("untrusted identity or credential forwarded")
		}
		var request struct {
			ID     string `json:"id"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request.Params.Name == "project.create" && r.Header.Get("X-Project-Id") != "" {
			t.Error("create received fake project")
		}
		if request.Params.Name == "project.open" && r.Header.Get("X-Project-Id") != projectID {
			t.Error("project scope missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}, "structuredContent": map[string]any{"ok": true}}})
	}))
	defer svc.Close()
	g := NewGatewayWithClient(nil)
	g.services["project"] = ServiceRegistration{Domain: "project", Endpoint: svc.URL, Tools: []ToolDefinition{{Name: "project.create"}, {Name: "project.open"}}}
	t.Setenv("STUDIO_BEARER_TOKEN", "never-forward-this")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", "")
	if _, status, _ := g.CallVerified(context.Background(), "project.create", map[string]any{}, p, ""); status != 503 {
		t.Fatal("fell back to shared token", status)
	}
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("s", 32))
	if _, status, _ := g.CallTool(context.Background(), "project.create", nil, map[string]string{"X-User-Id": p.SubjectID}); status != 403 {
		t.Fatal(status)
	}
	if _, status, e := g.CallVerified(context.Background(), "project.create", map[string]any{}, p, ""); status != 200 || e != nil {
		t.Fatal(status, e)
	}
	if _, status, e := g.CallVerified(context.Background(), "project.open", map[string]any{"project_id": projectID}, p, projectID); status != 200 || e != nil {
		t.Fatal(status, e)
	}
	before := calls
	if _, status, _ := g.CallVerified(context.Background(), "project.open", map[string]any{"project_id": projectID}, p, id()); status != 400 {
		t.Fatal("scope mismatch accepted", status)
	}
	if _, status, _ := g.CallVerified(context.Background(), "project.unknown", map[string]any{"project_id": projectID}, p, ""); status != 404 {
		t.Fatal("unregistered action accepted", status)
	}
	if calls != before {
		t.Fatal("rejected call reached service")
	}
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	r := g.services["project"]
	r.Endpoint = redirect.URL
	g.services["project"] = r
	if _, status, _ := g.CallVerified(context.Background(), "project.create", map[string]any{}, p, ""); status != 502 || redirected {
		t.Fatal("redirect leaked internal identity")
	}
}

func TestUserBearerIsNeverADownstreamCredential(t *testing.T) {
	t.Setenv("STUDIO_BEARER_TOKEN", "external-user-token")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "")
	for _, domain := range []string{"image", "video", "music"} {
		t.Setenv("STUDIO_"+strings.ToUpper(domain)+"_TOKEN", "")
		t.Setenv(strings.ToUpper(domain)+"_MCP_BEARER_TOKEN", "")
		if got := NewGatewayWithClient(nil).resolveToken(domain); got != "" {
			t.Fatal("user bearer used as internal credential")
		}
	}
}
