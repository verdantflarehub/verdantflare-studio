package mcp

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVideoContentTransfer(t *testing.T) {
	t.Setenv("STUDIO_VIDEO_TOKEN", "internal-video")
	t.Setenv("VIDEO_MCP_BEARER_TOKEN", "")
	t.Setenv("STUDIO_BEARER_TOKEN", "external-studio")
	path := "/artifacts/art_" + strings.Repeat("a", 32) + "/content"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != path || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer internal-video" || r.Header.Get("Cookie") != "" || r.Header.Get("X-User-Id") != "verified-user" {
			t.Error("incorrect destination or leaked credentials")
		}
		if v, ok := r.Header["Range"]; ok && (len(v) != 1 || v[0] == "") {
			t.Error("empty or duplicate range")
		}
		http.ServeContent(w, r, "clip.mp4", time.Time{}, strings.NewReader("video-bytes"))
	}))
	defer upstream.Close()
	g := NewGatewayWithClient(nil)
	g.services["video"] = ServiceRegistration{Endpoint: upstream.URL + "/mcp", Tools: []ToolDefinition{{Name: "video.result"}}}
	for _, tc := range []struct {
		rangeHeader, body string
		status            int
	}{{"", "video-bytes", 200}, {"bytes=0-4", "video", 206}, {"bytes=500-", "", 416}} {
		r, status, err := g.VideoContent(t.Context(), path, tc.rangeHeader, map[string]string{"X-User-Id": "verified-user", "Cookie": "secret", "Authorization": "Bearer external-studio"})
		if err != nil || status != tc.status {
			t.Fatalf("transfer: %d %v", status, err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if status != 416 && string(b) != tc.body {
			t.Fatal("content changed")
		}
	}
	for _, bad := range []string{"/v2/artifacts/id/content", "/runtime-artifacts/art_" + strings.Repeat("a", 32) + "/content", path + "?url=http://other", strings.Replace(path, "art_", "../", 1)} {
		if _, status, err := g.VideoContent(t.Context(), bad, "", nil); status != 400 || err == nil {
			t.Fatal("invalid route accepted", bad)
		}
	}
	t.Setenv("STUDIO_VIDEO_TOKEN", "")
	if _, status, _ := g.VideoContent(t.Context(), path, "", nil); status != 503 || calls != 3 {
		t.Fatal("external credential used as fallback")
	}
	t.Setenv("STUDIO_VIDEO_TOKEN", "internal-video")
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	svc := g.services["video"]
	svc.Endpoint = redirect.URL + "/mcp"
	g.services["video"] = svc
	if _, status, _ := g.VideoContent(t.Context(), path, "", nil); status != 502 || redirected {
		t.Fatal("redirect followed")
	}
	delete(g.services, "video")
	if _, status, _ := g.VideoContent(t.Context(), path, "", nil); status != 503 {
		t.Fatal("removed registration still served")
	}
}
