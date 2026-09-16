package web

import (
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSessionAndBoundary(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/identity/login":
			w.WriteHeader(201)
			io.WriteString(w, `{"access_token":"test-private-credential","user_id":"u1"}`)
		case "/catalog/apps":
			if r.Header.Get("Authorization") != "Bearer test-private-credential" {
				t.Error("missing server credential")
			}
			io.WriteString(w, `{"items":[]}`)
		case "/identity/logout":
			w.WriteHeader(204)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	station, _ := application.NewStation(upstream.URL)
	handler := New(station, "https://studio.example", fstest.MapFS{"index.html": {Data: []byte("market")}})
	call := func(method, path, origin, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	if rr := call("POST", "/studio/api/login", "https://evil.example", "{}", nil); rr.Code != 403 {
		t.Fatal(rr.Code)
	}
	if calls != 0 {
		t.Fatal("CSRF reached upstream")
	}
	if rr := call("GET", "/studio/api/apps", "", "", nil); rr.Code != 401 {
		t.Fatal(rr.Code)
	}
	rr := call("POST", "/studio/api/login", "https://studio.example", `{"username":"admin","password":"test"}`, nil)
	if rr.Code != 201 || strings.Contains(rr.Body.String(), "credential") {
		t.Fatal("credential disclosure or login failed")
	}
	cookie := rr.Result().Cookies()[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Value == "test-private-credential" {
		t.Fatal("unsafe cookie")
	}
	if rr = call("GET", "/studio/api/apps", "", "", cookie); rr.Code != 200 {
		t.Fatal(rr.Code)
	}
	if rr = call("GET", "/studio/api/apps/../../identity", "", "", cookie); rr.Code != 404 {
		t.Fatal("path escape", rr.Code)
	}
	if rr = call("POST", "/studio/api/commands", "https://studio.example", strings.Repeat("x", 16385), cookie); rr.Code != 413 {
		t.Fatal(rr.Code)
	}
	if rr = call("POST", "/studio/api/logout", "https://studio.example", "", cookie); rr.Code != 204 {
		t.Fatal(rr.Code)
	}
	if rr = call("GET", "/studio/api/apps", "", "", cookie); rr.Code != 401 {
		t.Fatal("logout did not invalidate session")
	}
	if rr = call("GET", "/studio/", "", "", nil); rr.Code != 200 || rr.Body.String() != "market" {
		t.Fatal("asset route failed")
	}
}
