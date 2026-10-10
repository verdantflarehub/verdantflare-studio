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

func TestEgressGatewayMethodsAndBody(t *testing.T) {
	const id = "019a0000-0000-7000-8000-000000000001"
	seen := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/login" {
			w.WriteHeader(201)
			io.WriteString(w, `{"access_token":"fixture-token"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing identity")
		}
		raw, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.Path+" "+string(raw))
		if r.Method == "DELETE" {
			w.WriteHeader(204)
		} else {
			io.WriteString(w, `{"item":{"proxy_id":"fixture"}}`)
		}
	}))
	defer upstream.Close()
	station, _ := application.NewStation(upstream.URL)
	handler := New(station, "https://studio.example", fstest.MapFS{})
	call := func(method, path, origin, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	login := call("POST", "/api/login", "https://studio.example", `{}`, nil)
	cookie := login.Result().Cookies()[0]
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		path := "/api/proxies"
		if method == "PUT" || method == "DELETE" {
			path += "/" + id
		}
		body := ""
		if method == "POST" || method == "PUT" {
			body = `{"note":"must reach Core","port":7891}`
		}
		w := call(method, path, "https://studio.example", body, cookie)
		if w.Code != 200 && w.Code != 204 {
			t.Fatal(method, w.Code)
		}
	}
	if !strings.Contains(seen[2], `"note":"must reach Core"`) {
		t.Fatal("PUT body dropped", seen)
	}
	before := len(seen)
	for _, method := range []string{"POST", "PUT", "DELETE"} {
		if w := call(method, "/api/proxies/"+id, "https://evil.example", "{}", cookie); w.Code != 403 {
			t.Fatal("CSRF accepted", method, w.Code)
		}
	}
	if len(seen) != before {
		t.Fatal("CSRF reached Core")
	}
	if w := call("GET", "/api/proxies", "", "", nil); w.Code != 401 {
		t.Fatal("unauthenticated read")
	}
	if w := call("PUT", "/api/proxies/"+id, "https://studio.example", strings.Repeat("x", 16385), cookie); w.Code != 413 {
		t.Fatal("unbounded PUT")
	}
	if w := call("POST", "/api/proxies/"+id+"/assign", "https://studio.example", "{}", cookie); w.Code != 404 {
		t.Fatal("H2 assignment exposed")
	}
}
