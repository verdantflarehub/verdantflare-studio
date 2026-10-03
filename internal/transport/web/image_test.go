package web

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestImageIsolation(t *testing.T) {
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
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != "Bearer image-secret" {
			t.Error("missing image auth")
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
	router, s := NewServer(station, "https://studio.example", assets)
	s.EnableImage(router, ImageConfig{upstream.URL, "image-secret"})

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

	login := func() *http.Cookie {
		return call("POST", "/api/login", "https://studio.example", nil).Result().Cookies()[0]
	}

	// 1. Static dashboard access requires no auth
	if w := call("GET", "/apps/image/dashboard", "", nil); w.Code != 200 {
		t.Fatalf("expected 200 for static dashboard, got %d", w.Code)
	}

	// 2. Unauthenticated API access rejected with 401
	if w := call("GET", "/apps/image/api/tasks", "https://studio.example", nil); w.Code != 401 {
		t.Fatalf("expected 401 for unauthenticated API, got %d", w.Code)
	}

	// 3. Authenticated admin access succeeds
	cookie := login()
	if w := call("GET", "/apps/image/api/tasks", "https://studio.example", cookie); w.Code != 200 {
		t.Fatalf("expected 200 for authenticated API, got %d", w.Code)
	}
}
