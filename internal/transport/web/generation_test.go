package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

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
