package web

import (
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestVideoIsolation(t *testing.T) {
	role := "admin"
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
		w.Write([]byte(`{"roles":["` + role + `"]}`))
	}))
	defer core.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
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
	w := call("POST", "/studio/api/login", "https://studio.example", nil)
	cookie := w.Result().Cookies()[0]
	if call("GET", "/studio/apps/video/api/dashboard", "", nil).Code != 401 {
		t.Fatal("anonymous data access")
	}
	if call("GET", "/studio/apps/video/api/dashboard", "", cookie).Code != 200 {
		t.Fatal("authorized read failed")
	}
	for _, path := range []string{"/runtime-artifacts/x/content", "/mcp", "/api/../identity/me", "/dashboard/static/no.js"} {
		before := calls
		if call("GET", "/studio/apps/video"+path, "", cookie).Code != 404 || calls != before {
			t.Fatal("allowlist bypass", path)
		}
	}
	if call("POST", "/studio/apps/video/api/tasks", "https://evil.example", cookie).Code != 403 {
		t.Fatal("CSRF")
	}
	role = "viewer"
	if call("GET", "/studio/apps/video/api/dashboard", "", cookie).Code != 403 {
		t.Fatal("nonadmin access")
	}
	role = "admin"
	revoked = true
	if call("GET", "/studio/apps/video/api/dashboard", "", cookie).Code != 401 {
		t.Fatal("revoked session")
	}
	w = call("GET", "/studio/apps/video/dashboard", "", nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'none'") {
		t.Fatal("static CSP")
	}
}
