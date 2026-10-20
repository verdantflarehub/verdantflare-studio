package mcp

import (
	"context"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorldDraftTransferScope(t *testing.T) {
	id := "01900000-0000-7000-8000-000000000001"
	path := "/v2/artifacts/" + id + "/content"
	base := "store_id=" + id + "&artifact_id=" + id
	good := base + "&world_draft_id=" + id + "&world_draft_revision_id=" + id
	if project, err := TransferScope("GET", path, good); err != nil || project != "" {
		t.Fatal("valid draft read requires a fake project")
	}
	for _, query := range []string{
		base + "&world_draft_id=" + id, base + "&world_draft_revision_id=" + id,
		good + "&project_id=" + id + "&project_revision_id=" + id,
		good + "&asset_id=" + id + "&asset_version_id=" + id,
		good + "&world_draft_id=" + id, good + "&url=https://example.com/content",
	} {
		if _, err := TransferScope("GET", path, query); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
}

func TestWorldDraftGatewayHasNoImplicitProject(t *testing.T) {
	const id = "01900000-0000-7000-8000-000000000001"
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", strings.Repeat("w", 32))
	g := NewGatewayWithClient(nil)
	g.services["world"] = ServiceRegistration{Domain: "world", Endpoint: "https://world.example/mcp", Tools: []ToolDefinition{{Name: "world.draft"}}}
	called := 0
	g.httpClient = &http.Client{Transport: contentTransport(func(r *http.Request) (*http.Response, error) {
		called++
		if r.Header.Get("X-Project-Id") != "" || r.Header.Get("X-User-Id") != id || r.Header.Get("X-Organization-Id") != id || r.URL.Host != "world.example" {
			t.Fatal("draft identity or project scope changed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"` + id + `","result":{"content":[]}}`))}, nil
	})}
	p := project.Principal{OrganizationID: id, SubjectID: id, RequestID: id}
	args := map[string]any{"mode": "create", "commit_id": id, "name": "Reference", "purpose": "collection", "subjects": []string{}}
	if _, status, err := g.CallVerified(context.Background(), "world.draft", args, p, ""); err != nil || status != 200 {
		t.Fatal(status, err)
	}
	if _, status, _ := g.CallVerified(context.Background(), "world.draft", args, p, id); status != 400 || called != 1 {
		t.Fatal("stale project scope attached to draft")
	}
	delete(g.services, "world")
	if _, status, _ := g.CallVerified(context.Background(), "world.draft", args, p, ""); status != 404 || called != 1 {
		t.Fatal("unavailable draft service fell back")
	}
}

func TestWorldDraftRegistrationGatewayScope(t *testing.T) {
	const id = "01900000-0000-7000-8000-000000000001"
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", strings.Repeat("w", 32))
	g := NewGatewayWithClient(nil)
	g.services["world"] = ServiceRegistration{Domain: "world", Endpoint: "https://world.example/mcp", Tools: []ToolDefinition{{Name: "world.register"}}}
	calls := 0
	expectedProject := ""
	g.httpClient = &http.Client{Transport: contentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("X-Project-Id") != expectedProject {
			t.Fatal("incorrect publication project scope")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"` + id + `","result":{"content":[]}}`))}, nil
	})}
	p := project.Principal{OrganizationID: id, SubjectID: id, RequestID: id}
	for _, bad := range []map[string]any{
		{"source_draft_id": id, "source_project_id": id},
		{"source_draft_id": id, "source_project_id": ""},
		{"source_draft_id": ""}, {"source_draft_id": 123},
	} {
		if _, status, _ := g.CallVerified(context.Background(), "world.register", bad, p, ""); status != 400 {
			t.Fatal("mixed or malformed source accepted", status)
		}
	}
	args := map[string]any{"source_draft_id": id, "source_revision_id": id}
	if _, status, _ := g.CallVerified(context.Background(), "world.register", args, p, id); status != 400 || calls != 0 {
		t.Fatal("stale project header accepted")
	}
	if _, status, err := g.CallVerified(context.Background(), "world.register", args, p, ""); err != nil || status != 200 || calls != 1 {
		t.Fatal("independent registration rejected", status, err)
	}
	expectedProject = id
	if _, status, err := g.CallVerified(context.Background(), "world.register", map[string]any{"source_project_id": id, "source_revision_id": id}, p, id); err != nil || status != 200 || calls != 2 {
		t.Fatal("legacy project registration broken", status, err)
	}
}
