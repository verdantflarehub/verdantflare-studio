package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestAppCenterWebCommandGates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw, err := os.ReadFile("../../appcenter/testdata/commands.generated.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Valid bool                  `json:"valid"`
		Input appcenter.CommandCore `json:"input"`
	}
	if json.Unmarshal(raw, &vectors) != nil {
		t.Fatal("vectors")
	}
	inputs := map[string]any{
		"/studio/appcenter/v1/commands":         appcenter.CommandSubmit{SchemaVersion: 1, Prepared: vectors[0].Input},
		"/studio/appcenter/v1/commands/prepare": appcenter.CommandPrepare{SchemaVersion: 1, RequestID: "trace", IdempotencyKey: "fixed-key", CardKey: "catalog:sample", ActionID: "install", ExpectedBindingRevision: "sha256:" + strings.Repeat("a", 64)},
	}
	for path, input := range inputs {
		for _, scenario := range []string{"valid", "query", "duplicate", "unknown", "no-origin", "no-cookie", "wrong-media", "too-large"} {
			t.Run(path+"/"+scenario, func(t *testing.T) {
				r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
				s.sessions["test-cookie"] = session{token: "test-core-session", expires: time.Now().Add(time.Hour)}
				stub := &appCenterWebStub{}
				s.EnableAppCenter(r, stub)
				encoded, _ := json.Marshal(input)
				body, target := string(encoded), path
				switch scenario {
				case "query":
					target += "?scope=other"
				case "duplicate":
					body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
				case "unknown":
					body = strings.TrimSuffix(body, "}") + `,"url":"https://other.invalid"}`
				case "too-large":
					body += strings.Repeat(" ", 16<<10)
				}
				req := httptest.NewRequest("POST", target, strings.NewReader(body))
				req.Header.Set("Origin", "https://studio.invalid")
				req.Header.Set("Content-Type", "application/json")
				if scenario != "no-cookie" {
					req.AddCookie(&http.Cookie{Name: cookieName, Value: "test-cookie"})
				}
				if scenario == "no-origin" {
					req.Header.Del("Origin")
				}
				if scenario == "wrong-media" {
					req.Header.Set("Content-Type", "text/plain")
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if scenario == "valid" {
					if w.Code != 200 || stub.calls != 1 {
						t.Fatalf("valid rejected: %d", w.Code)
					}
				} else if w.Code == 200 || stub.calls != 0 {
					t.Fatal("invalid reached application")
				}
			})
		}
	}
}

type appCenterWebStub struct{ calls int }

func (s *appCenterWebStub) Prepare(ctx context.Context, token string, _ appcenter.CommandPrepare) application.Result {
	return s.Get(ctx, token)
}
func (s *appCenterWebStub) Submit(ctx context.Context, token string, _ appcenter.CommandSubmit) application.Result {
	return s.Get(ctx, token)
}

func (s *appCenterWebStub) Open(ctx context.Context, token string, _ appcenter.OpenRequest) application.Result {
	return s.Get(ctx, token)
}

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

func TestAppCenterWebOpenGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, scenario := range []string{"valid", "no-origin", "cross-site", "query", "wrong-mime", "unknown", "duplicate", "oversize", "missing-field", "no-cookie"} {
		t.Run(scenario, func(t *testing.T) {
			r, s := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
			s.sessions["test-cookie"] = session{token: "test-core-session", expires: time.Now().Add(time.Hour)}
			stub := &appCenterWebStub{}
			s.EnableAppCenter(r, stub)
			input := appcenter.OpenRequest{SchemaVersion: 1, CardKey: "installation:sample", EntryID: "dashboard", ExpectedViewRevision: "sha256:" + strings.Repeat("a", 64), ExpectedBindingRevision: "sha256:" + strings.Repeat("b", 64)}
			raw, _ := json.Marshal(input)
			body := string(raw)
			path := "/studio/appcenter/v1/open"
			switch scenario {
			case "query":
				path += "?app=other"
			case "unknown":
				body = strings.TrimSuffix(body, "}") + `,"url":"https://outside.invalid"}`
			case "duplicate":
				body = strings.Replace(body, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
			case "oversize":
				body += strings.Repeat(" ", 4096)
			case "missing-field":
				body = `{}`
			}
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("Origin", "https://studio.invalid")
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(&http.Cookie{Name: cookieName, Value: "test-cookie"})
			switch scenario {
			case "no-origin":
				req.Header.Del("Origin")
			case "cross-site":
				req.Header.Set("Sec-Fetch-Site", "cross-site")
			case "wrong-mime":
				req.Header.Set("Content-Type", "text/plain")
			case "no-cookie":
				req.Header.Del("Cookie")
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if scenario == "valid" {
				if w.Code != 200 || stub.calls != 1 {
					t.Fatal("valid open rejected", w.Code)
				}
			} else if w.Code == 200 || stub.calls != 0 {
				t.Fatal("invalid open accepted", w.Code)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response cacheable")
			}
		})
	}
}
