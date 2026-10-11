package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

type blenderTransport func(*http.Request) (*http.Response, error)

func (f blenderTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestBlenderContentBoundary(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org, requestID, uploadID := id(), id(), id(), id()
	t.Setenv("STUDIO_BLENDER_CONTENT_TOKEN", "content-internal")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", "project-internal")
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", "artifact-internal")
	var received *http.Request
	var body []byte
	redirect := false
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Clone(r.Context())
		body = nil
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		w.Header().Set("Set-Cookie", "private=secret")
		w.Header().Set("X-Internal-Secret", "private")
		if redirect {
			w.Header().Set("Location", "https://untrusted.example")
			w.WriteHeader(302)
			_, _ = w.Write([]byte("private redirect body"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	})
	available := true
	handler := blenderContentHandlerWithTransport(func(domain string) (string, bool) {
		if domain != "project" && domain != "artifact" {
			t.Errorf("unexpected domain %q", domain)
		}
		return "https://" + domain + ".example:8443", available
	}, blenderTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Port() != "8443" {
			t.Fatal("business request used MCP port")
		}
		w := httptest.NewRecorder()
		upstream.ServeHTTP(w, r)
		return w.Result(), nil
	}))
	call := func(method, path string, content []byte, change func(*http.Request)) *httptest.ResponseRecorder {
		received = nil
		r := httptest.NewRequest(method, path, bytes.NewReader(content))
		r.Header.Set("Authorization", "Bearer content-internal")
		r.Header.Set("X-User-Id", user)
		r.Header.Set("X-Organization-Id", org)
		r.Header.Set("X-Request-Id", requestID)
		r.Header.Set("Cookie", "external=secret")
		r.Header.Set("X-Forwarded-Host", "untrusted.example")
		r.Header.Set("X-Project-Id", "spoofed")
		if change != nil {
			change(r)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"POST", "/mcp", 404}, {"POST", "/project/delete", 404},
		{"GET", "/project/open", 404}, {"POST", "/project/open?target=other", 400},
		{"POST", "/project/%6fpen", 400}, {"PUT", "/artifact/uploads/../../mcp/content", 404},
		{"GET", "/artifact/versions/" + uploadID, 400},
		{"POST", "/artifact/uploads/" + uuid.NewString() + "/commit", 404},
	} {
		w := call(tc.method, tc.path, nil, nil)
		if w.Code != tc.status || received != nil {
			t.Fatalf("route %s %s: %d", tc.method, tc.path, w.Code)
		}
	}
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong") },
		func(r *http.Request) { r.Header.Add("X-User-Id", user) },
		func(r *http.Request) { r.Header.Set("X-Organization-Id", uuid.NewString()) },
	} {
		if w := call("POST", "/project/open", nil, change); w.Code != 401 || received != nil {
			t.Fatal("untrusted caller accepted")
		}
	}
	w := call("POST", "/project/open", []byte(`{"project_id":"fixture"}`), nil)
	if w.Code != 200 || received.URL.Path != "/internal/v1/project/open" || received.Header.Get("Authorization") != "Bearer project-internal" {
		t.Fatal("project forwarding failed")
	}
	w = call("POST", "/project/list", []byte(`{}`), nil)
	if w.Code != 200 || received.URL.Path != "/internal/v1/project/list" || received.Header.Get("Authorization") != "Bearer project-internal" {
		t.Fatal("project selection forwarding failed")
	}
	w = call("POST", "/project/create", []byte(`{}`), nil)
	if w.Code != 200 || received.URL.Path != "/internal/v1/project/create" || received.Header.Get("Authorization") != "Bearer project-internal" {
		t.Fatal("project creation forwarding failed")
	}
	payload := bytes.Repeat([]byte{0, 1, 255, 13, 10}, 300000)
	w = call("PUT", "/artifact/uploads/"+uploadID+"/content", payload, nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), payload) || !bytes.Equal(body, payload) {
		t.Fatal("binary stream changed")
	}
	if received.URL.Path != "/v2/artifacts/uploads/"+uploadID+"/content" || received.Header.Get("Authorization") != "Bearer artifact-internal" || received.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatal("artifact forwarding failed")
	}
	if received.Header.Get("X-User-Id") != user || received.Header.Get("X-Organization-Id") != org || received.Header.Get("X-Request-Id") != requestID {
		t.Fatal("identity missing")
	}
	for _, h := range []string{"Cookie", "X-Forwarded-Host", "X-Project-Id"} {
		if received.Header.Get(h) != "" {
			t.Fatal("client header leaked", h)
		}
	}
	for _, h := range []string{"Set-Cookie", "X-Internal-Secret"} {
		if w.Header().Get(h) != "" {
			t.Fatal("server header leaked", h)
		}
	}
	redirect = true
	w = call("POST", "/artifact/uploads", nil, nil)
	if w.Code != 502 || strings.Contains(w.Body.String(), "private") || w.Header().Get("Location") != "" {
		t.Fatal("redirect exposed")
	}
	available = false
	if w = call("POST", "/project/open", nil, nil); w.Code != 503 || received != nil {
		t.Fatal("missing service not rejected")
	}
}

func TestBlenderRuntimePrivateBridge(t *testing.T) {
	user, org, trace := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	t.Setenv("STUDIO_BLENDER_CONTENT_TOKEN", "content-private")
	t.Setenv("STUDIO_BLENDER_CONTROL_TOKEN", "control-private")
	calls := 0
	expectedPath := "/internal/v1/blender/create"
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != expectedPath || r.Header.Get("Authorization") != "Bearer control-private" || r.Header.Get("X-User-Id") != user || r.Header.Get("X-Organization-Id") != org || r.Header.Get("Cookie") != "" {
			t.Error("invalid control forwarding")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"phase":"awaiting_content"}`)
	})
	t.Setenv("STATION_CORE_URL", "https://core.example:8443")
	h := blenderContentHandlerWithTransport(func(string) (string, bool) { t.Error("control route used MCP discovery"); return "", false }, blenderTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		upstream.ServeHTTP(w, r)
		return w.Result(), nil
	}))
	call := func(token, path string) int {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-User-Id", user)
		r.Header.Set("X-Organization-Id", org)
		r.Header.Set("X-Request-Id", trace)
		r.Header.Set("Cookie", "external=secret")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if status := call("browser-token", "/runtime/instances/create"); status != 401 || calls != 0 {
		t.Fatal("public token admitted", status)
	}
	if status := call("content-private", "/runtime/instances/create"); status != 200 || calls != 1 {
		t.Fatal("private create bridge failed", status)
	}
	if status := call("content-private", "/runtime/instances/create?target=other"); status != 400 || calls != 1 {
		t.Fatal("target override allowed", status)
	}
	if status := call("content-private", "/runtime/instances/delete"); status != 404 || calls != 1 {
		t.Fatal("arbitrary action allowed", status)
	}
	expectedPath = "/internal/v1/blender/options"
	if status := call("browser-token", "/runtime/options"); status != 401 || calls != 1 {
		t.Fatal("public options token admitted")
	}
	if status := call("content-private", "/runtime/options"); status != 200 || calls != 2 {
		t.Fatal("options bridge failed", status)
	}
	expectedPath = "/internal/v1/blender/start"
	if status := call("browser-token", "/runtime/instances/start"); status != 401 || calls != 2 {
		t.Fatal("public start token admitted", status)
	}
	if status := call("content-private", "/runtime/instances/start"); status != 200 || calls != 3 {
		t.Fatal("private start bridge failed", status)
	}
	if status := call("content-private", "/runtime/instances/start?target=other"); status != 400 || calls != 3 {
		t.Fatal("start target overridden", status)
	}
	expectedPath = "/internal/v1/blender/access"
	if status := call("browser-token", "/runtime/instances/access"); status != 401 || calls != 3 {
		t.Fatal("public access token admitted", status)
	}
	if status := call("content-private", "/runtime/instances/access"); status != 200 || calls != 4 {
		t.Fatal("private access bridge failed", status)
	}
	if status := call("content-private", "/runtime/instances/access?kind=gui"); status != 400 || calls != 4 {
		t.Fatal("access query override admitted", status)
	}
	expectedPath = "/internal/v1/blender/stop"
	if status := call("browser-token", "/runtime/instances/stop"); status != 401 || calls != 4 {
		t.Fatal("public stop token admitted", status)
	}
	if status := call("content-private", "/runtime/instances/stop"); status != 200 || calls != 5 {
		t.Fatal("private stop bridge failed", status)
	}
	if status := call("content-private", "/runtime/instances/stop?target=other"); status != 400 || calls != 5 {
		t.Fatal("stop target override admitted", status)
	}
	expectedPath = "/internal/v1/blender/destroy"
	if status := call("browser-token", "/runtime/instances/destroy"); status != 401 || calls != 5 {
		t.Fatal("public destroy token admitted", status)
	}
	if status := call("content-private", "/runtime/instances/destroy"); status != 200 || calls != 6 {
		t.Fatal("private destroy bridge failed", status)
	}
	if status := call("content-private", "/runtime/instances/destroy?target=other"); status != 400 || calls != 6 {
		t.Fatal("destroy target override admitted", status)
	}
}

func TestBlenderSplitBusinessOriginsFailClosed(t *testing.T) {
	t.Setenv("STUDIO_CONTENT_SERVICE_MODE", "split")
	t.Setenv("STUDIO_BLENDER_CONTENT_TOKEN", "content-private")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_ARTIFACT_SERVICE_TOKEN", strings.Repeat("a", 32))
	gateway := mcp.NewGatewayWithClient(nil)
	const id = "01900000-0000-7000-8000-000000000001"
	calls := 0
	h := blenderContentHandlerWithTransport(gateway.BusinessEndpoint, blenderTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Scheme != "https" || r.URL.Port() != "8443" || r.Header.Get("X-User-Id") != id || r.Header.Get("X-Organization-Id") != id || r.Header.Get("X-Request-Id") != id {
			t.Fatal("business origin or authenticated identity changed")
		}
		want := strings.Repeat("p", 32)
		if r.URL.Hostname() == "artifact.example" {
			want = strings.Repeat("a", 32)
		}
		if r.Header.Get("Authorization") != "Bearer "+want {
			t.Fatal("business credential mixed")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	}))
	call := func(path string) int {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer content-private")
		for _, name := range []string{"X-User-Id", "X-Organization-Id", "X-Request-Id"} {
			r.Header.Set(name, id)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	for _, tc := range []struct{ env, path, origin string }{
		{"STUDIO_PROJECT_SERVICE_URL", "/project/open", "https://project.example:8443"},
		{"STUDIO_ARTIFACT_URL", "/artifact/uploads", "https://artifact.example:8443"},
	} {
		for _, invalid := range []string{"", tc.origin + "/mcp", "http://private.example:8443", tc.origin + "?"} {
			t.Setenv(tc.env, invalid)
			before := calls
			if status := call(tc.path); status != 503 || calls != before {
				t.Fatal("invalid business origin reached network", status)
			}
		}
		t.Setenv(tc.env, tc.origin)
		if status := call(tc.path); status != 200 {
			t.Fatal("explicit business origin unavailable", status)
		}
	}
	if calls != 2 {
		t.Fatal("unexpected business requests")
	}
}
