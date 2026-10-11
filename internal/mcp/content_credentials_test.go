package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestContentCredentialsPreserveLegacyAndSeparateSplit(t *testing.T) {
	g := NewGatewayWithClient(nil)
	project := strings.Repeat("p", 32)
	world := strings.Repeat("w", 32)
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", project)
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", world)
	for _, mode := range []string{"", "legacy", "split"} {
		t.Setenv("STUDIO_CONTENT_SERVICE_MODE", mode)
		if g.resolveToken("project") != project {
			t.Fatal("Project credential changed")
		}
		want := project
		if mode == "split" {
			want = world
		}
		if g.resolveToken("world") != want {
			t.Fatal("World credential mode mismatch")
		}
	}
}

type contentTransport func(*http.Request) (*http.Response, error)

func (f contentTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestContentSplitDiscoveryPreservesDomainEndpointAndIdentity(t *testing.T) {
	g := NewGatewayWithClient(nil)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", strings.Repeat("w", 32))
	g.services["project"] = ServiceRegistration{Domain: "project", Endpoint: "https://project.example/mcp", Tools: []ToolDefinition{{Name: "project.list"}}}
	g.services["world"] = ServiceRegistration{Domain: "world", Endpoint: "https://world.example/mcp", Tools: []ToolDefinition{{Name: "world.list"}}}
	const id = "019a00b0-0000-7000-8000-000000000001"
	p := project.Principal{OrganizationID: id, SubjectID: id, RequestID: id}
	calls := []string{}
	g.httpClient = &http.Client{Transport: contentTransport(func(r *http.Request) (*http.Response, error) {
		var request struct {
			ID     string `json:"id"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("invalid MCP envelope")
		}
		domain := strings.SplitN(request.Params.Name, ".", 2)[0]
		calls = append(calls, domain)
		want := strings.Repeat("p", 32)
		if domain == "world" {
			want = strings.Repeat("w", 32)
		}
		if r.URL.Host != domain+".example" || r.Header.Get("Authorization") != "Bearer "+want || r.Header.Get("X-Organization-Id") != id || r.Header.Get("X-User-Id") != id || r.Header.Get("X-Request-Id") != id {
			t.Fatal("domain endpoint, credential or identity mixed")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":"` + id + `","result":{"content":[]}}`))}, nil
	})}
	for _, name := range []string{"project.list", "world.list"} {
		if _, status, err := g.CallVerified(context.Background(), name, map[string]any{}, p, ""); err != nil || status != 200 {
			t.Fatal(status, err)
		}
	}
	delete(g.services, "world")
	if _, status, _ := g.CallVerified(context.Background(), "world.list", map[string]any{}, p, ""); status != 404 || len(calls) != 2 {
		t.Fatal("missing World fell back to Project")
	}
}

func TestContentCredentialsSplitHasNoSharedFallback(t *testing.T) {
	g := NewGatewayWithClient(nil)
	project := strings.Repeat("p", 32)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", project)
	t.Setenv("INTERNAL_SERVICE_TOKEN", "must-not-fallback")
	for _, world := range []string{"", project, "short", strings.Repeat("w", 32) + "\n"} {
		t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", world)
		if g.resolveToken("world") != "" || g.resolveToken("project") != "" {
			t.Fatal("incomplete or shared split credentials accepted")
		}
	}
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", strings.Repeat("w", 32))
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "unknown")
	if g.resolveToken("world") != "" {
		t.Fatal("unknown mode fell back")
	}
}
