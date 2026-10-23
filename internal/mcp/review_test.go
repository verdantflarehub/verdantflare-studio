package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestReviewMCPBindsNestedScopeAndNeverInteractiveOrigin(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	projectID := id()
	g := NewGatewayWithClient(nil)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "legacy")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("s", 32))
	for _, domain := range []string{"project", "world"} {
		g.services[domain] = ServiceRegistration{Domain: domain, Endpoint: "http://content.invalid/mcp", Tools: []ToolDefinition{
			{Name: domain + ".review_open"}, {Name: domain + ".review_append"}, {Name: domain + ".review_status"},
		}}
	}
	calls := 0
	g.httpClient.Transport = contentTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		var input struct {
			ID     string `json:"id"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.NewDecoder(req.Body).Decode(&input) != nil {
			t.Fatal("invalid request")
		}
		wantScope := ""
		if strings.HasPrefix(input.Params.Name, "project.") {
			wantScope = projectID
		}
		if req.Header.Get("X-Review-Origin") != "agent" || req.Header.Get("X-Project-Id") != wantScope || req.Header.Get("X-User-Id") != p.SubjectID {
			t.Fatal("review authority changed in transit")
		}
		body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": input.ID, "result": map[string]any{"structuredContent": map[string]any{}}})
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	for _, name := range []string{"project.review_open", "project.review_append"} {
		args := map[string]any{"target_locator": map[string]any{"kind": "project", "project_id": projectID}}
		for _, header := range []string{"", projectID} {
			if _, status, err := g.CallVerified(context.Background(), name, args, p, header); err != nil || status != 200 {
				t.Fatal(status, err)
			}
		}
	}
	for _, name := range []string{"world.review_open", "world.review_append", "world.review_status"} {
		if _, status, err := g.CallVerified(context.Background(), name, map[string]any{}, p, ""); err != nil || status != 200 {
			t.Fatal(status, err)
		}
	}
	if _, status, err := g.CallVerified(context.Background(), "project.review_status", map[string]any{"project_id": projectID}, p, projectID); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	before := calls
	for _, tc := range []struct {
		name   string
		args   map[string]any
		header string
	}{
		{"project.review_open", map[string]any{}, ""},
		{"project.review_open", map[string]any{"project_id": projectID}, ""},
		{"project.review_append", map[string]any{"target_locator": map[string]any{"kind": "world", "project_id": projectID}}, ""},
		{"project.review_append", map[string]any{"target_locator": map[string]any{"kind": "project", "project_id": projectID}}, id()},
		{"project.review_open", map[string]any{"target_locator": map[string]any{"kind": "project", "project_id": projectID}, "project_id": projectID}, ""},
		{"world.review_open", map[string]any{}, projectID},
		{"world.review_append", map[string]any{}, projectID},
		{"world.review_status", map[string]any{}, projectID},
	} {
		if _, status, _ := g.CallVerified(context.Background(), tc.name, tc.args, p, tc.header); status != 400 {
			t.Fatal("invalid scope accepted", tc.name, status)
		}
	}
	if calls != before {
		t.Fatal("invalid scope reached service")
	}
	// Even an internal caller's header map cannot elevate the MCP channel.
	if _, status, err := g.call(context.Background(), "project.review_open", nil, map[string]string{"X-User-Id": p.SubjectID, "X-Project-Id": projectID, "X-Request-Id": p.RequestID, "X-Review-Origin": "human_interactive"}); err != nil || status != 200 {
		t.Fatal(status, err)
	}
}
