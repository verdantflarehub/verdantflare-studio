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

type appCenterWebStub struct{ calls int }

func (s *appCenterWebStub) Get(_ context.Context, token string) application.Result {
	s.calls++
	if token != "test-core-session" {
		panic("incorrect server credential")
	}
	return application.Result{Status: 200, Data: json.RawMessage(`{"items":[]}`), RequestID: "test-request"}
}
func TestAppCenterWebReadGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []string{"valid", "query", "body", "cross-site", "foreign-origin", "bearer", "duplicate-cookie", "no-cookie"} {
		t.Run(scenario, func(t *testing.T) {
			r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
			s.sessions["test-cookie"] = session{token: "test-core-session", expires: time.Now().Add(time.Hour)}
			stub := &appCenterWebStub{}
			s.EnableAppCenter(r, stub)
			path, body := "/studio/appcenter/v1/view", ""
			if scenario == "query" {
				path += "?organization=other"
			}
			if scenario == "body" {
				body = "{}"
			}
			req := httptest.NewRequest("GET", path, strings.NewReader(body))
			req.AddCookie(&http.Cookie{Name: cookieName, Value: "test-cookie"})
			switch scenario {
			case "cross-site":
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			case "foreign-origin":
				req.Header.Set("Origin", "https://foreign.invalid")
			case "bearer":
				req.Header.Set("Authorization", "Bearer arbitrary")
			case "duplicate-cookie":
				req.AddCookie(&http.Cookie{Name: cookieName, Value: "other-cookie"})
			case "no-cookie":
				req.Header.Del("Cookie")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if scenario == "valid" {
				if w.Code != 200 || stub.calls != 1 {
					t.Fatal("valid request rejected", w.Code)
				}
			} else if w.Code == 200 || stub.calls != 0 {
				t.Fatal("invalid request reached application")
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
		})
	}
}
