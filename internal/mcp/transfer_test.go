package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestTransferKeepsCallerOwnedUploadFileOpen(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	f, err := os.CreateTemp(t.TempDir(), "upload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = f.WriteString("source bytes"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != "source bytes" || r.ContentLength != 12 {
			t.Error("upload changed")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("a", 32))
	g := NewGatewayWithClient(nil)
	g.services["artifact"] = ServiceRegistration{Domain: "artifact", Endpoint: server.URL + "/mcp", Tools: []ToolDefinition{{Name: "artifact.write"}}}
	response, status, err := g.Transfer(t.Context(), p, "PUT", "/v2/artifacts/uploads/"+id()+"/content", "", f, 12)
	if err != nil || status != 200 {
		t.Fatal(status, err)
	}
	response.Body.Close()
	if _, err = f.Stat(); err != nil {
		t.Fatalf("caller file closed: %v", err)
	}
}

func TestTransferRouteAndCredentials(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	path := "/v2/artifacts/" + id() + "/content"
	query := "store_id=" + id() + "&artifact_id=" + id()
	for _, tc := range []struct{ method, path, query string }{
		{"POST", "/v2/artifacts/retentions", ""}, {"GET", path, query + "&store_id=" + id()},
		{"GET", path, query + "&project_id=" + id()}, {"GET", path, query + "&url=https://other.example"},
		{"PUT", "/v2/artifacts/uploads/../../retentions", ""},
	} {
		if _, e := TransferScope(tc.method, tc.path, tc.query); e == nil {
			t.Fatal("unsafe route accepted")
		}
	}
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("a", 32))
	called := 0
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.URL.Path != path || r.URL.RawQuery != query || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 32) || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Error("incorrect transfer identity or path")
		}
		w.Header().Set("Content-Length", "5")
		_, _ = io.WriteString(w, "media")
	}))
	defer downstream.Close()
	g := NewGatewayWithClient(nil)
	g.services["artifact"] = ServiceRegistration{Domain: "artifact", Endpoint: downstream.URL + "/mcp", Tools: []ToolDefinition{{Name: "artifact.read"}}}
	response, status, e := g.Transfer(context.Background(), p, "GET", path, query, nil, 0)
	if e != nil || status != 200 {
		t.Fatal(status, e)
	}
	b, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(b) != "media" {
		t.Fatal("content changed")
	}
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", "")
	if _, status, _ := g.Transfer(context.Background(), p, "GET", path, query, nil, 0); status != 503 || called != 1 {
		t.Fatal("missing credential reached service")
	}
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("a", 32))
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	r := g.services["artifact"]
	r.Endpoint = redirect.URL + "/mcp"
	g.services["artifact"] = r
	if _, status, _ := g.Transfer(context.Background(), p, "GET", path, query, nil, 0); status != 502 || redirected {
		t.Fatal("redirect exposed internal credentials")
	}
}
