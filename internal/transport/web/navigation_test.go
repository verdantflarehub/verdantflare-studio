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
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

type navigationWebStub struct {
	calls int
	token string
	patch application.NavigationPreferencePatch
}

func (s *navigationWebStub) Get(_ context.Context, token string) application.Result {
	s.calls++
	s.token = token
	return application.Result{Status: 200, Data: json.RawMessage(`{"projection":{"entries":[]},"preferenceRevision":"0"}`)}
}
func (s *navigationWebStub) Patch(ctx context.Context, token string, patch application.NavigationPreferencePatch) application.Result {
	s.patch = patch
	return s.Get(ctx, token)
}
func navigationWebFixture() (*gin.Engine, *navigationWebStub) {
	gin.SetMode(gin.TestMode)
	r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
	s.sessions["browser-session"] = session{token: "server-owned-session", expires: time.Now().Add(time.Hour)}
	stub := &navigationWebStub{}
	s.EnableNavigation(r, stub)
	return r, stub
}

const navigationPatchJSON = `{"appId":"sample","entryId":"main","pinned":false,"order":0,"preferenceRevision":"0"}`

func TestNavigationWebClosedCookieGate(t *testing.T) {
	for _, kind := range []string{"get", "post", "missing-origin", "foreign-origin", "duplicate-origin", "bearer", "encoding", "duplicate-cookie", "missing-cookie", "duplicate-field", "unknown-field", "missing-pinned", "query", "cross-site", "get-body", "wrong-content-type"} {
		t.Run(kind, func(t *testing.T) {
			r, stub := navigationWebFixture()
			method, path, body := "POST", "/studio/navigation/preferences", navigationPatchJSON
			if kind == "get" || kind == "get-body" {
				method, path, body = "GET", "/studio/navigation", ""
			}
			if kind == "get-body" {
				body = "{}"
			}
			switch kind {
			case "duplicate-field":
				body = strings.Replace(body, `"pinned":false`, `"pinned":false,"pinned":true`, 1)
			case "unknown-field":
				body = strings.Replace(body, `"pinned":false`, `"pinned":false,"system":true`, 1)
			case "missing-pinned":
				body = strings.Replace(body, `"pinned":false,`, "", 1)
			case "query":
				path += "?userId=foreign"
			}
			req := dashboardWebRequest(method, path, body)
			switch kind {
			case "missing-origin":
				req.Header.Del("Origin")
			case "foreign-origin":
				req.Header.Set("Origin", "https://foreign.invalid")
			case "duplicate-origin":
				req.Header.Add("Origin", "https://studio.invalid")
			case "bearer":
				req.Header.Set("Authorization", "Bearer arbitrary")
			case "encoding":
				req.Header.Set("Content-Encoding", "gzip")
			case "duplicate-cookie":
				req.AddCookie(&http.Cookie{Name: cookieName, Value: "other"})
			case "missing-cookie":
				req.Header.Del("Cookie")
			case "cross-site":
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			case "wrong-content-type":
				req.Header.Set("Content-Type", "text/plain")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if kind == "get" || kind == "post" {
				if w.Code != 200 || stub.calls != 1 || stub.token != "server-owned-session" {
					t.Fatal("valid cookie request rejected", w.Code)
				}
				if kind == "post" && (stub.patch.Pinned || stub.patch.Order != 0) {
					t.Fatal("explicit zero/false lost")
				}
			} else if w.Code < 400 || stub.calls != 0 {
				t.Fatal("unsafe request reached service", w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private projection cacheable")
			}
		})
	}
}
