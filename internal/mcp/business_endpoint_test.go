package mcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestBusinessEndpointSeparatesConfiguredOriginsFromMCP(t *testing.T) {
	g := NewGatewayWithClient(nil)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	for _, tc := range []struct{ domain, env string }{
		{"project", "STUDIO_PROJECT_SERVICE_URL"},
		{"world", "STUDIO_WORLD_SERVICE_URL"},
		{"artifact", "STUDIO_ARTIFACT_URL"},
	} {
		mcp := "https://" + tc.domain + ".example:8444/mcp"
		origin := "https://" + tc.domain + ".example:8443"
		g.services[tc.domain] = ServiceRegistration{Domain: tc.domain, Endpoint: mcp}
		t.Setenv(tc.env, origin+"/")
		if got, ok := g.BusinessEndpoint(tc.domain); !ok || got != origin {
			t.Fatal("explicit business origin not selected")
		}
		if got, ok := g.ServiceEndpoint(tc.domain); !ok || got != mcp {
			t.Fatal("MCP discovery endpoint changed")
		}
		t.Setenv(tc.env, "")
		if _, ok := g.BusinessEndpoint(tc.domain); ok {
			t.Fatal("missing business origin fell back to MCP or another domain")
		}
	}
	if _, ok := g.BusinessEndpoint("station-core"); ok {
		t.Fatal("unexpected business domain accepted")
	}
}

func TestBusinessEndpointRejectsUnsafeAndAmbiguousOrigins(t *testing.T) {
	g := NewGatewayWithClient(nil)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	for _, raw := range []string{
		"", "https://", "https://:8443", "https://project.example:0", "https://project.example:65536", "https://project.example:",
		"https://project.example/mcp", "https://project.example//", "https://project.example/%2f", "https://project.example?", "https://project.example?q=1",
		"https://project.example#", "https://project.example#fragment", "https://user:secret@project.example", "https:project.example",
		"http://project.example", "http://localhost", "http://10.0.0.1", "http://[::]", "https://project.example\n", " https://project.example",
	} {
		t.Setenv("STUDIO_PROJECT_SERVICE_URL", raw)
		if _, ok := g.BusinessEndpoint("project"); ok {
			t.Fatal("unsafe business origin accepted")
		}
	}
	for _, raw := range []string{"https://project.example", "https://project.example:8443", "http://127.0.0.1:8091", "http://[::1]:8091"} {
		t.Setenv("STUDIO_PROJECT_SERVICE_URL", raw)
		if got, ok := g.BusinessEndpoint("project"); !ok || got != raw {
			t.Fatal("controlled origin rejected")
		}
	}
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "unknown")
	if _, ok := g.BusinessEndpoint("project"); ok {
		t.Fatal("unknown mode fell back")
	}
}

func TestBusinessEndpointLegacyUsesOriginalSharedListener(t *testing.T) {
	g := NewGatewayWithClient(nil)
	g.services["project"] = ServiceRegistration{Endpoint: "http://project.internal:8091/mcp"}
	t.Setenv("STUDIO_PROJECT_SERVICE_URL", "https://must-not-use.example:8443")
	for _, mode := range []string{"", "legacy"} {
		t.Setenv("STUDIO_CONTENT_SERVICE_MODE", mode)
		if got, ok := g.BusinessEndpoint("project"); !ok || got != "http://project.internal:8091" {
			t.Fatal("legacy shared listener changed")
		}
		if _, ok := g.BusinessEndpoint("world"); ok {
			t.Fatal("missing legacy domain fell back")
		}
	}
	for _, raw := range []string{"https://project.internal/business", "https://project.internal/mcp?", "https://project.internal/%6dcp", "https://project.internal/mcp#"} {
		g.services["project"] = ServiceRegistration{Endpoint: raw}
		if _, ok := g.BusinessEndpoint("project"); ok {
			t.Fatal("ambiguous legacy MCP endpoint accepted")
		}
	}
}

func TestLegacyArtifactExplicitBusinessListener(t *testing.T) {
	g := NewGatewayWithClient(nil)
	g.services["artifact"] = ServiceRegistration{Endpoint: "https://artifact.example:8194/mcp"}
	for _, mode := range []string{"", "legacy"} {
		t.Setenv("STUDIO_CONTENT_SERVICE_MODE", mode)
		t.Setenv("STUDIO_ARTIFACT_URL", "https://artifact.example:8094")
		if origin, ok := g.BusinessEndpoint("artifact"); !ok || origin != "https://artifact.example:8094" {
			t.Fatal("Artifact transfer inherited MCP listener")
		}
		for _, bad := range []string{"http://artifact.example:8094", "https://artifact.example:8094/mcp", "https://user:secret@artifact.example"} {
			t.Setenv("STUDIO_ARTIFACT_URL", bad)
			if _, ok := g.BusinessEndpoint("artifact"); ok {
				t.Fatal("invalid business endpoint fell back")
			}
		}
		t.Setenv("STUDIO_ARTIFACT_URL", "")
		if origin, ok := g.BusinessEndpoint("artifact"); !ok || origin != "https://artifact.example:8194" {
			t.Fatal("unconfigured legacy compatibility changed")
		}
	}
}

func TestBusinessTransferUsesArtifactBusinessPortAndIdentity(t *testing.T) {
	g := NewGatewayWithClient(nil)
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_ARTIFACT_URL", "https://artifact.example:8443")
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("a", 32))
	g.services["artifact"] = ServiceRegistration{Domain: "artifact", Endpoint: "https://artifact.example:8444/mcp", Tools: []ToolDefinition{{Name: "artifact.read"}, {Name: "artifact.write"}}}
	const id = "01900000-0000-7000-8000-000000000001"
	p := project.Principal{SubjectID: id, OrganizationID: id, RequestID: id}
	path := "/v2/artifacts/uploads/" + id + "/content"
	calls := 0
	g.httpClient.Transport = contentTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "artifact.example:8443" || r.URL.Path != path || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 32) || r.Header.Get("X-User-Id") != id || r.Header.Get("X-Organization-Id") != id || r.Header.Get("X-Request-Id") != id {
			t.Fatal("artifact business destination, credential or identity changed")
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil || string(payload) != "content" || r.ContentLength != 7 {
			t.Fatal("binary upload changed")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	response, status, err := g.Transfer(context.Background(), p, "PUT", path, "", strings.NewReader("content"), 7)
	if err != nil || status != 200 || calls != 1 {
		t.Fatal("business transfer failed", status, err)
	}
	response.Body.Close()
	for _, origin := range []string{"", "http://artifact.example:8443", "https://artifact.example:8444/mcp"} {
		t.Setenv("STUDIO_ARTIFACT_URL", origin)
		if _, status, err := g.Transfer(context.Background(), p, "PUT", path, "", strings.NewReader("content"), 7); err == nil || status != 503 || calls != 1 {
			t.Fatal("invalid business origin fell back or reached network")
		}
	}
	t.Setenv("STUDIO_ARTIFACT_URL", "https://artifact.example:8443")
	delete(g.services, "artifact")
	if _, status, err := g.Transfer(context.Background(), p, "PUT", path, "", strings.NewReader("content"), 7); err == nil || status != 503 || calls != 1 {
		t.Fatal("binary route skipped discovery availability")
	}
}
