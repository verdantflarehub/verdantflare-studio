package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
)

type preinstalledWebStub struct {
	reads, writes int
	token         string
	intent        preinstalled.Intent
}

func (s *preinstalledWebStub) Get(_ context.Context, token string) application.Result {
	s.reads++
	s.token = token
	return application.Result{Status: 200, Data: []byte(`{"enabled":false,"configDigest":"","items":[]}`)}
}
func (s *preinstalledWebStub) Execute(_ context.Context, token string, in preinstalled.Intent) application.Result {
	s.writes++
	s.token = token
	s.intent = in
	return application.Result{Status: 200, Data: []byte(`{"enabled":true,"configDigest":"digest","items":[]}`)}
}

func TestPreinstalledWebReadOnlyAndClosedCookieIntent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, kind := range []string{"get", "execute", "adopt", "origin", "bearer", "execute-field", "duplicate-intent", "missing-reinstall", "get-body", "query"} {
		t.Run(kind, func(t *testing.T) {
			r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
			s.sessions["browser-session"] = session{token: "server-owned-session", expires: time.Now().Add(time.Hour)}
			stub := &preinstalledWebStub{}
			s.EnablePreinstalled(r, stub)
			method, path, body := "POST", "/studio/preinstalled/reconcile", `{"intentId":"019a00b0-0000-7000-8000-000000000001","retry":[],"reinstall":[],"adopt":{}}`
			switch kind {
			case "get", "get-body":
				method, path, body = "GET", "/studio/preinstalled", ""
				if kind == "get-body" {
					body = "{}"
				}
			case "adopt":
				body = strings.TrimSuffix(body, "}") + `,"adoptTemplateDigests":{"resources":"sha256:` + strings.Repeat("c", 64) + `"}}`
			case "execute-field":
				body = strings.TrimSuffix(body, "}") + `,"Execute":true}`
			case "duplicate-intent":
				body = strings.TrimSuffix(body, "}") + `,"intentId":"changed"}`
			case "missing-reinstall":
				body = strings.Replace(body, `"reinstall":[],`, "", 1)
			case "query":
				path += "?appId=resources"
			}
			req := dashboardWebRequest(method, path, body)
			if kind == "origin" {
				req.Header.Set("Origin", "https://foreign.invalid")
			}
			if kind == "bearer" {
				req.Header.Set("Authorization", "Bearer browser-selected")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			switch kind {
			case "get":
				if w.Code != 200 || stub.reads != 1 || stub.writes != 0 {
					t.Fatal("GET executed intent", w.Code)
				}
			case "execute", "adopt":
				if w.Code != 200 || stub.writes != 1 || stub.reads != 0 || stub.intent.Execute || stub.token != "server-owned-session" {
					t.Fatal("invalid execution gate", w.Code)
				}
			default:
				if w.Code < 400 || stub.reads+stub.writes != 0 {
					t.Fatal("unsafe request reached service", w.Code)
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private state cacheable")
			}
		})
	}
}
