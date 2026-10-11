package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

type dashboardWebStub struct {
	t       *testing.T
	calls   []string
	request dashboard.AppRequest
}

func (s *dashboardWebStub) PublicInstanceAsset(_ context.Context, artifact, path string) ([]byte, string, application.Result) {
	s.calls = append(s.calls, "public:"+artifact+":"+path)
	return []byte("export const version = 1;"), "text/javascript", application.Result{Status: 200}
}

func (s *dashboardWebStub) Installations(_ context.Context, token string) application.Result {
	s.record(token, "installations")
	return application.Result{Status: 200, Data: json.RawMessage(`{"items":[]}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) Inspect(_ context.Context, token, id string) application.Result {
	s.record(token, "inspect:"+id)
	return application.Result{Status: 200, Data: json.RawMessage(`{"launch":{},"context":{}}`), RequestID: "request-test"}
}

func (s *dashboardWebStub) record(token, action string) {
	s.t.Helper()
	if token != "server-owned-session" {
		s.t.Fatal("unverified token")
	}
	s.calls = append(s.calls, action)
}
func (s *dashboardWebStub) Open(_ context.Context, token, id string) application.Result {
	s.record(token, "open:"+id)
	return application.Result{Status: 200, Data: json.RawMessage(`{"launch":{},"context":{}}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) Asset(_ context.Context, token, id, path string) ([]byte, string, application.Result) {
	s.record(token, "asset:"+id+":"+path)
	return []byte("<!doctype html><p>app</p>"), "text/html; charset=utf-8", application.Result{Status: 200, RequestID: "request-test"}
}
func (s *dashboardWebStub) Request(_ context.Context, token, id string, in dashboard.AppRequest) application.Result {
	s.record(token, "request:"+id)
	s.request = in
	return application.Result{Status: 409, Data: json.RawMessage(`{"code":"INSTANCE_BUSY"}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) Close(_ context.Context, token, id string) application.Result {
	s.record(token, "close:"+id)
	return application.Result{Status: 204, RequestID: "request-test"}
}

func (s *dashboardWebStub) OpenInstance(_ context.Context, token, id string, in dashboard.InstanceOpen) application.Result {
	s.record(token, "instance:"+id+":"+in.InstanceID+":"+in.EntryID)
	return application.Result{Status: 200, Data: json.RawMessage(`{}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) InspectWorkspace(_ context.Context, token, id string) application.Result {
	s.record(token, "workspace:"+id)
	return application.Result{Status: 200, Data: json.RawMessage(`{}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) WorkspaceAsset(_ context.Context, token, id, path string) ([]byte, string, application.Result) {
	s.record(token, "workspace-asset:"+id+":"+path)
	return []byte("<!doctype html>"), "text/html", application.Result{Status: 200, RequestID: "request-test"}
}
func (s *dashboardWebStub) WorkspaceRequest(_ context.Context, token, id string, in dashboard.AppRequest) application.Result {
	s.record(token, "workspace-request:"+id)
	s.request = in
	return application.Result{Status: 200, Data: json.RawMessage(`{}`), RequestID: "request-test"}
}
func (s *dashboardWebStub) CloseWorkspace(_ context.Context, token, id string) application.Result {
	s.record(token, "workspace-close:"+id)
	return application.Result{Status: 204, RequestID: "request-test"}
}

func dashboardWebFixture(t *testing.T) (*gin.Engine, *dashboardWebStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
	s.sessions["browser-session"] = session{token: "server-owned-session", expires: time.Now().Add(time.Hour)}
	stub := &dashboardWebStub{t: t}
	s.EnableDashboards(r, stub)
	return r, stub
}

func dashboardWebRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: cookieName, Value: "browser-session"})
	if method != "GET" {
		req.Header.Set("Origin", "https://studio.invalid")
	}
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestDashboardInstanceRoutesSeparatePrivateOperationsFromPublicReleaseAssets(t *testing.T) {
	r, stub := dashboardWebFixture(t)
	id := uuid.NewString()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/studio/dashboards/" + id + "/instances/open", `{"instanceId":"one","entryId":"editor"}`, 200},
		{"GET", "/studio/instance-workspaces/" + id, "", 200},
		{"GET", "/studio/instance-workspaces/" + id + "/assets/vf/workspace.html", "", 200},
		{"POST", "/studio/instance-workspaces/" + id + "/requests", `{"method":"POST","path":"/close","body":{"session_id":"editor-lease"}}`, 200},
		{"DELETE", "/studio/instance-workspaces/" + id, "", 204},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, dashboardWebRequest(tc.method, tc.path, tc.body))
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
	}
	if len(stub.calls) != 5 || !strings.HasPrefix(stub.calls[4], "workspace-close:") {
		t.Fatal("workspace called dashboard close")
	}
	for _, body := range []string{`{"instanceId":"one","entryId":"editor","url":"https://outside.invalid"}`, `{"instanceId":"one","entryId":"editor","entryId":"shell"}`} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, dashboardWebRequest("POST", "/studio/dashboards/"+id+"/instances/open", body))
		if w.Code != 400 || len(stub.calls) != 5 {
			t.Fatal("untrusted entry reached service")
		}
	}
	request := httptest.NewRequest("GET", "/studio/instance-assets/"+strings.Repeat("a", 64)+"/vf/editor.mjs", nil)
	request.Header.Set("Origin", "null")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("opaque renderer cannot read public code", w.Code)
	}
	request = httptest.NewRequest("GET", "/studio/instance-workspaces/"+id, nil)
	request.Header.Set("Origin", "null")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code < 400 || len(stub.calls) != 6 {
		t.Fatal("public code access granted private workspace access")
	}
}

func TestDashboardWebSessionRoutesAreGenericAndPreserveSemantics(t *testing.T) {
	r, stub := dashboardWebFixture(t)
	id, installation := uuid.NewString(), uuid.Must(uuid.NewV7()).String()
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/studio/dashboard-installations", "", 200},
		{"GET", "/studio/dashboards/" + id, "", 200},
		{"POST", "/studio/dashboards", `{"installationId":"` + installation + `"}`, 200},
		{"GET", "/studio/dashboards/" + id + "/assets/dashboard/index.html", "", 200},
		{"POST", "/studio/dashboards/" + id + "/requests", `{"method":"POST","path":"/instances/two/start","body":{"profile":null}}`, 409},
		{"DELETE", "/studio/dashboards/" + id, "", 204},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, dashboardWebRequest(tc.method, tc.path, tc.body))
		if w.Code != tc.status || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") != "request-test" {
			t.Fatal("bad transport result", w.Code)
		}
		if strings.Contains(tc.path, "/assets/") && !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox;") {
			t.Fatal("standalone page can execute")
		}
		if tc.status == 204 && w.Body.Len() != 0 {
			t.Fatal("204 has body")
		}
	}
	if len(stub.calls) != 6 || stub.request.Path != "/instances/two/start" || string(stub.request.Body) != `{"profile":null}` {
		t.Fatal("business request changed")
	}
}

func TestDashboardWebRejectsUntrustedRequestsBeforeService(t *testing.T) {
	id := uuid.NewString()
	installation := uuid.Must(uuid.NewV7()).String()
	for name, change := range map[string]func(*http.Request){
		"missing origin":   func(r *http.Request) { r.Header.Del("Origin") },
		"foreign origin":   func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") },
		"duplicate origin": func(r *http.Request) { r.Header.Add("Origin", "https://studio.invalid") },
		"cross site":       func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"missing cookie":   func(r *http.Request) { r.Header.Del("Cookie") },
		"duplicate cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: cookieName, Value: "browser-session"}) },
		"forged bearer":    func(r *http.Request) { r.Header.Set("Authorization", "Bearer browser-input") },
		"wrong mime":       func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"duplicate mime":   func(r *http.Request) { r.Header.Add("Content-Type", "application/json") },
		"encoded body":     func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") },
		"query":            func(r *http.Request) { r.URL.RawQuery = "organization=other" },
		"empty query":      func(r *http.Request) { r.URL.ForceQuery = true },
	} {
		t.Run(name, func(t *testing.T) {
			r, stub := dashboardWebFixture(t)
			req := dashboardWebRequest("POST", "/studio/dashboards", `{"installationId":"`+installation+`"}`)
			change(req)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || len(stub.calls) != 0 {
				t.Fatal("untrusted request reached service", w.Code)
			}
		})
	}
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/studio/dashboards/" + id + "/assets/dashboard/%69ndex.html", ""},
		{"DELETE", "/studio/dashboards/" + id, `{}`},
		{"GET", "/studio/dashboards/" + id + "/assets/dashboard/index.html", `{}`},
		{"POST", "/studio/dashboards", `{"installationId":"` + installation + `","trusted":true}`},
		{"POST", "/studio/dashboards", `{"installationId":"` + installation + `","installationId":"` + installation + `"}`},
		{"POST", "/studio/dashboards", `{"installationId":"app-name"}`},
		{"POST", "/studio/dashboards/" + id + "/requests", `{"method":"POST","path":"/instances","body":{"x":1,"x":2}}`},
		{"POST", "/studio/dashboards/" + id + "/requests", `{"method":"GET","path":"/instances","upstream":"https://other.invalid"}`},
		{"DELETE", "/studio/dashboards/not-a-session", ""},
	} {
		r, stub := dashboardWebFixture(t)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, dashboardWebRequest(tc.method, tc.path, tc.body))
		if w.Code < 400 || len(stub.calls) != 0 {
			t.Fatal("invalid body or path reached service", w.Code)
		}
	}
}
