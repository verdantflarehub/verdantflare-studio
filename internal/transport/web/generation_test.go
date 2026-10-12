package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestGenerationMediaProxyFailsClosedWithoutControlService(t *testing.T) {
	t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
	forwarded := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++; _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer provider.Close()
	engine, s := NewServer(bearerStation(t, "fixture-token"), "https://studio.example", fstest.MapFS{}, VideoConfig{URL: provider.URL, Token: "internal-fixture"})
	s.EnableImage(engine, ImageConfig{URL: provider.URL, Token: "internal-fixture"})
	s.sessions["fixture-session"] = session{token: "fixture-token", expires: time.Now().Add(time.Hour)}
	for _, path := range []string{"/apps/image/api/tasks", "/studio/apps/image/api/tasks", "/apps/video/api/tasks", "/studio/apps/video/api/tasks"} {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"project_id":"not-a-real-project","prompt":"must not execute"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://studio.example")
		r.AddCookie(&http.Cookie{Name: cookieName, Value: "fixture-session"})
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatal("media proxy bypass", path, w.Code)
		}
	}
	if forwarded != 0 {
		t.Fatal("uncontrolled request reached provider")
	}
}

func TestGenerationApprovalRejectsBearerAndCrossOriginBeforeAuthentication(t *testing.T) {
	engine, _ := NewServer(nil, "https://studio.example", fstest.MapFS{})
	for _, entry := range []struct{ origin, site, bearer string }{
		{"https://studio.example", "same-origin", "Bearer ordinary-mcp-token"},
		{"https://other.example", "cross-site", ""},
		{"https://studio.example", "same-site", ""},
		{"", "", ""},
	} {
		r := httptest.NewRequest(http.MethodPost, "/studio/projects/generation/approve", strings.NewReader(`{}`))
		r.Header.Set("Origin", entry.origin)
		r.Header.Set("Sec-Fetch-Site", entry.site)
		if entry.bearer != "" {
			r.Header.Set("Authorization", entry.bearer)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("confirmation shortcut accepted", w.Code)
		}
	}
}
