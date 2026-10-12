package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestMediaProxyUsesExistingSessionAuthorityWithoutApproval(t *testing.T) {
	t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
	calls := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer internal-fixture" {
			t.Error("media proxy credential changed")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer provider.Close()
	engine, s := NewServer(bearerStation(t, "fixture-token"), "https://studio.example", fstest.MapFS{}, VideoConfig{URL: provider.URL, Token: "internal-fixture"})
	s.EnableImage(engine, ImageConfig{URL: provider.URL, Token: "internal-fixture"})
	s.sessions["fixture-session"] = session{token: "fixture-token", expires: time.Now().Add(time.Hour)}
	for _, route := range engine.Routes() {
		if route.Path == "/studio/projects/generation/approve" {
			t.Fatal("removed approval route remains registered")
		}
	}
	for _, path := range []string{"/apps/image/api/tasks", "/studio/apps/image/api/tasks", "/apps/video/api/tasks", "/studio/apps/video/api/tasks"} {
		for _, authenticated := range []bool{true, false} {
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"prompt":"isolated fixture"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", "https://studio.example")
			if authenticated {
				r.AddCookie(&http.Cookie{Name: cookieName, Value: "fixture-session"})
			}
			before := calls
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, r)
			if authenticated && (w.Code != 200 || calls != before+1) {
				t.Fatalf("authorized media proxy blocked: %s %d", path, w.Code)
			}
			if !authenticated && (w.Code < 400 || calls != before) {
				t.Fatal("unauthenticated media proxy accepted")
			}
		}
	}
}
