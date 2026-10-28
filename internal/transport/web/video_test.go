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
	upstreamStatus, upstreamType, upstreamBody := 200, "application/json", `{"ok":true}`
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
		w.Header().Set("Content-Type", upstreamType)
		w.Header().Set("ETag", `"video-fixture"`)
		w.WriteHeader(upstreamStatus)
		w.Write([]byte(upstreamBody))
	}))
	defer upstream.Close()
	station, _ := application.NewStation(core.URL)
	var assets fs.FS = fstest.MapFS{}
	router := New(station, "https://studio.example", assets, VideoConfig{upstream.URL, "video-secret"})
	call := func(method, path, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		r.Header.Set("X-User-Id", "spoofed")
		r.Header.Set("X-Organization-Id", "spoofed")
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
	if call("GET", "/apps/video/api/model-catalog", "", nil).Code != 401 {
		t.Fatal("anonymous catalog access")
	}
	if call("GET", "/apps/video/api/model-catalog", "", cookie).Code != 200 {
		t.Fatal("authorized catalog read failed")
	}
	for _, path := range []string{"/api/model-catalog/extra", "/dashboard/frontend/model-catalog-secret.js"} {
		if call("GET", "/apps/video"+path, "", cookie).Code != 404 {
			t.Fatal("catalog allowlist too broad", path)
		}
	}
	if call("POST", "/apps/video/api/model-catalog", "https://studio.example", cookie).Code != 404 {
		t.Fatal("catalog write accepted")
	}
	for _, path := range []string{"/dashboard/frontend/model-catalog.js", "/dashboard/frontend/model-catalog.css"} {
		if call("GET", "/apps/video"+path, "", nil).Code != 200 {
			t.Fatal("catalog asset denied", path)
		}
	}
	before = calls
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
	for _, path := range []string{"projects", "capabilities", "assets", "tasks", "tasks/task_1", "tasks/task_1/content"} {
		if call("GET", "/apps/video/api/higgsfield/"+path, "", cookie).Code != 200 {
			t.Fatal("member read denied", path)
		}
	}
	for _, path := range []string{"preflight", "tasks", "attempt", "imports/prepare", "imports/chunk", "imports/status", "imports/commit", "tasks/task_1/result"} {
		if call("POST", "/apps/video/api/higgsfield/"+path, "https://studio.example", cookie).Code != 200 {
			t.Fatal("member action denied", path)
		}
	}
	for _, path := range []string{"account", "limits", "auth", "tasks/task_1/delete"} {
		if call("GET", "/apps/video/api/higgsfield/"+path, "", cookie).Code != 404 {
			t.Fatal("member boundary bypass", path)
		}
	}
	if call("POST", "/apps/video/api/higgsfield/tasks", "https://evil.example", cookie).Code != 403 || call("GET", "/apps/video/api/higgsfield/tasks", "", nil).Code != 401 {
		t.Fatal("member session or origin bypass")
	}
	if call("GET", "/apps/video/api/dashboard", "", cookie).Code != 403 {
		t.Fatal("nonadmin access")
	}
	role = "admin"
	for _, code := range []string{"authorization_required", "limits_unavailable", "account_unavailable"} {
		upstreamStatus = 503
		upstreamBody = `{"error":{"code":"` + code + `","message":"Specific channel failure","source":"higgsfield","retryable":true,"accepted":false,"http_status":503},"debug":"private provider body"}`
		out := call("POST", "/apps/video/api/higgsfield/preflight", "https://studio.example", cookie)
		if out.Code != 503 || !strings.Contains(out.Body.String(), code) || !strings.Contains(out.Body.String(), `"accepted":false`) || strings.Contains(out.Body.String(), "private provider body") {
			t.Fatal("channel business error was lost or leaked", out.Code, out.Body.String())
		}
	}
	upstreamBody = `{"error":{"code":"submission_unknown","message":"Recover original task; Authorization=Bearer-secret HTTPS://provider.example/result?key=secret /var/private/key C:\\private\\key","source":"grok","retryable":false,"accepted":true,"http_status":502,"task_id":"video_task_original","debug":"secret"}}`
	out := call("POST", "/apps/video/api/tasks", "https://studio.example", cookie)
	if out.Code != 503 || !strings.Contains(out.Body.String(), `"accepted":true`) || !strings.Contains(out.Body.String(), "video_task_original") || strings.Contains(out.Body.String(), "private") || strings.Contains(out.Body.String(), "secret") || strings.Contains(out.Body.String(), "provider.example") {
		t.Fatal("recovery identity lost or unsafe diagnostic exposed", out.Body.String())
	}
	for _, body := range []string{`<html>private proxy failure</html>`, `{"error":{"code":"provider_failed","message":"private","source":"unknown","retryable":false}}`, strings.Repeat("x", 65537)} {
		upstreamBody = body
		out := call("POST", "/apps/video/api/tasks", "https://studio.example", cookie)
		if out.Code != 502 || out.Body.Len() != 0 {
			t.Fatal("unrecognized upstream body crossed boundary", out.Code)
		}
	}
	upstreamBody, upstreamType = `{"error":{"code":"provider_failed","message":"private","source":"grok","retryable":false}}`, "text/html"
	if out := call("POST", "/apps/video/api/tasks", "https://studio.example", cookie); out.Code != 502 || out.Body.Len() != 0 {
		t.Fatal("non-JSON upstream body crossed boundary")
	}
	upstreamStatus, upstreamType, upstreamBody = 200, "application/json", `{"ok":true}`
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
