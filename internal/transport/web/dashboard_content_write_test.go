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
)

type dashboardWriterStub struct {
	*dashboardWebStub
	input application.DashboardTextWrite
}

func (s *dashboardWriterStub) WriteContent(_ context.Context, token, id string, in application.DashboardTextWrite, _ application.DashboardContentGateway) application.Result {
	s.record(token, "write:"+id)
	s.input = in
	return application.Result{Status: 409, RequestID: "request-test", Data: json.RawMessage(`{"code":"IDEMPOTENCY_CONFLICT"}`)}
}

func TestDashboardContentWriteRouteUsesSessionGateAndClosedInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, server := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
	server.sessions["browser-session"] = session{token: "server-owned-session", expires: time.Now().Add(time.Hour)}
	stub := &dashboardWriterStub{dashboardWebStub: &dashboardWebStub{t: t}}
	server.EnableDashboards(r, stub)
	path := "/studio/dashboards/" + uuid.NewString() + "/capabilities/artifact.write"
	id := uuid.Must(uuid.NewV7()).String()
	body := `{"mode":"text","write_id":"` + id + `","source":{"kind":"user_edit","world_draft_id":"` + id + `"},"mime":"text/plain","text":"正文"}`
	w := httptest.NewRecorder()
	r.ServeHTTP(w, dashboardWebRequest("POST", path, body))
	if w.Code != 409 || !strings.Contains(w.Body.String(), "IDEMPOTENCY_CONFLICT") || len(stub.calls) != 1 || stub.input.Text != "正文" || stub.input.Source.WorldDraftID != id {
		t.Fatal("lost write identity or original result", w.Code)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Request-ID") != "request-test" {
		t.Fatal("private result was not scoped to the request")
	}
	for name, modify := range map[string]func(*http.Request){
		"missing session":  func(r *http.Request) { r.Header.Del("Cookie") },
		"foreign origin":   func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") },
		"forged token":     func(r *http.Request) { r.Header.Set("Authorization", "Bearer untrusted") },
		"foreign identity": func(r *http.Request) { r.URL.RawQuery = "user=other" },
	} {
		t.Run(name, func(t *testing.T) {
			req := dashboardWebRequest("POST", path, body)
			modify(req)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || len(stub.calls) != 1 {
				t.Fatal("untrusted request reached writer")
			}
		})
	}
	for _, invalid := range []string{
		strings.Replace(body, `"text":"正文"`, `"text":"正文","user_id":"other"`, 1),
		strings.Replace(body, `"text":"正文"`, `"text":"first","text":"second"`, 1),
		strings.Replace(body, `"mode":"text"`, `"mode":"prepare"`, 1),
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, dashboardWebRequest("POST", path, invalid))
		if w.Code != 400 || len(stub.calls) != 1 {
			t.Fatal("invalid content reached writer")
		}
	}
}

func TestDashboardContentWriteUnavailableDoesNotFallBackToAppRequest(t *testing.T) {
	r, stub := dashboardWebFixture(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, dashboardWebRequest("POST", "/studio/dashboards/"+uuid.NewString()+"/capabilities/artifact.write", `{}`))
	if w.Code != 503 || len(stub.calls) != 0 {
		t.Fatal("unsupported writer invoked another service")
	}
}
