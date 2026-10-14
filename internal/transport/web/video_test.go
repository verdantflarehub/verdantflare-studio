package web

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestVideoIsolation(t *testing.T) {
	role := "admin"
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org := id(), id()
	revoked := false
	calls := 0
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/identity/login" {
			w.WriteHeader(201)
			w.Write([]byte(`{"access_token":"core-secret"}`))
			return
		}
		if revoked {
			w.WriteHeader(401)
			w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer core-secret" {
			t.Error("missing core auth")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"station_id": id(), "session_id": id(), "user_id": user, "organization_id": org, "organization_name": "fixture", "username": "fixture", "roles": []string{role}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute), "expires_at": time.Now().Add(time.Hour), "policy_version": 1, "revocation_version": 0, "request_id": "fixture"})
	}))
	defer core.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasPrefix(r.URL.Path, "/api/") && (r.Header.Get("X-User-Id") != user || r.Header.Get("X-Organization-Id") != org) {
			t.Error("missing verified actor")
		}
		if values, ok := r.Header["Range"]; ok && (len(values) == 0 || values[0] == "") {
			t.Error("empty Range header breaks Starlette FileResponse")
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != "Bearer video-secret" {
			t.Error("missing video auth")
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("host cookie leaked")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"video-fixture"`)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	station, _ := application.NewStation(core.URL)
	var assets fs.FS = fstest.MapFS{}
	router := New(station, "https://studio.example", assets, VideoConfig{upstream.URL, "video-secret"})
	call := func(method, path, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/api/login", "https://studio.example", nil)
	cookie := w.Result().Cookies()[0]
	if call("GET", "/apps/video/api/dashboard", "", nil).Code != 401 {
		t.Fatal("anonymous data access")
	}
	if call("GET", "/apps/video/api/dashboard", "", cookie).Code != 200 {
		t.Fatal("authorized read failed")
	}
	if call("GET", "/studio/apps/video/api/dashboard", "", cookie).Code != 200 {
		t.Fatal("authorized legacy read failed")
	}
	if w := call("GET", "/studio/apps/video/api/dashboard", "", cookie); w.Header().Get("ETag") != `"video-fixture"` {
		t.Fatal("gateway dropped the upstream entity validator")
	}
	if call("GET", "/apps/video/api/capabilities", "", nil).Code != 401 {
		t.Fatal("anonymous capabilities access")
	}
	if call("GET", "/apps/video/api/capabilities", "", cookie).Code != 200 {
		t.Fatal("authorized capabilities read failed")
	}
	if call("GET", "/apps/video/api/tasks/task_1/content", "", cookie).Code != 200 {
		t.Fatal("content read denied")
	}
	before := calls
	if call("POST", "/apps/video/api/capabilities", "https://studio.example", cookie).Code != 404 || calls != before {
		t.Fatal("capabilities must remain read only")
	}
	for _, path := range []string{"/api/capabilities/secrets", "/runtime-artifacts/x/content", "/mcp", "/api/../identity/me", "/dashboard/static/no.js", "/dashboard/frontend/no.js"} {
		before := calls
		if call("GET", "/apps/video"+path, "", cookie).Code != 404 || calls != before {
			t.Fatal("allowlist bypass", path)
		}
	}
	if call("POST", "/apps/video/api/tasks", "https://evil.example", cookie).Code != 403 {
		t.Fatal("CSRF")
	}
	role = "viewer"
	if call("GET", "/apps/video/api/dashboard", "", cookie).Code != 403 {
		t.Fatal("nonadmin access")
	}
	role = "admin"
	revoked = true
	if call("GET", "/apps/video/api/dashboard", "", cookie).Code != 401 {
		t.Fatal("revoked session")
	}
	w = call("GET", "/apps/video/dashboard", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Fatal("static CSP")
	}
	w = call("GET", "/apps/video/dashboard/frontend/studio-embed.js", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Fatal("frontend static CSP")
	}
	w = call("GET", "/studio/apps/video/dashboard", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Fatal("static legacy CSP")
	}
}
