package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestComfyUIEditorAuthenticationStaticAndRevocation(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	identity := map[string]any{"station_id": id(), "session_id": id(), "user_id": id(), "organization_id": id(), "organization_name": "test", "request_id": "test", "username": "test", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	revoked := false
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/identity/me" || r.Header.Get("Authorization") != "Bearer user-test" || revoked {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(identity)
	}))
	defer core.Close()
	calls := 0
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer editor-internal-service-token-0123456789" || r.Header.Get("Cookie") != "" {
			t.Error("credential boundary violated")
		}
		if strings.HasPrefix(r.URL.Path, "/internal/frontend/") {
			if r.Header.Get("X-User-Id") != "" {
				t.Error("static bytes should carry no personal identity")
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html>"))
			return
		}
		if r.Header.Get("X-User-Id") != identity["user_id"] || r.Header.Get("X-Organization-Id") != identity["organization_id"] {
			t.Error("unverified editor identity")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/renew") {
			w.WriteHeader(409)
			_, _ = w.Write([]byte(`{"code":"EDITOR_SESSION_EXPIRED"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events") {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte("{\"kind\":\"heartbeat\"}\n"))
			return
		}
		_, _ = w.Write([]byte(`{"session_id":"test-only","version":0}`))
	}))
	defer app.Close()
	station, _ := application.NewStation(core.URL)
	router, s := NewServer(station, "https://studio.example", fstest.MapFS{})
	s.comfyuiEndpoint = func() (string, bool) { return app.URL + "/internal/mcp", true }
	t.Setenv("STUDIO_COMFYUI_SERVICE_TOKEN", "editor-internal-service-token-0123456789")
	send := func(method, path, token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{"client_id":"test"}`))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		r.Header.Set("Cookie", "private-browser-cookie=secret")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/open", "user-test"); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/renew", "user-test"); w.Code != 409 || !strings.Contains(w.Body.String(), "SESSION_EXPIRED") {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/events", "user-test"); w.Code != 200 || w.Body.String() != "{\"kind\":\"heartbeat\"}\n" {
		t.Fatal(w.Code, w.Body.String())
	}
	before := calls
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/open", ""); w.Code != 403 || calls != before {
		t.Fatal("anonymous editor access", w.Code)
	}
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/open?target=other", "user-test"); w.Code != 400 || calls != before {
		t.Fatal("query escaped")
	}
	revoked = true
	if w := send("POST", "/studio/apps/comfyui/comfyuiA/editor/events", "user-test"); w.Code != 401 || calls != before {
		t.Fatal("revoked reconnect escaped", w.Code)
	}
	w := send("GET", "/apps/comfyui/static/?parent_origin=https%3A%2F%2Fstudio.example", "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("opaque static module access", w.Code)
	}
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src data:") || !strings.Contains(csp, "sandbox allow-scripts allow-downloads") || strings.Contains(csp, "allow-same-origin") {
		t.Fatal("unsafe iframe CSP", csp)
	}
	before = calls
	if w := send("GET", "/apps/comfyui/static/%2e%2e/private.json", ""); w.Code != 404 || calls != before {
		t.Fatal("static traversal escaped", w.Code)
	}
}
