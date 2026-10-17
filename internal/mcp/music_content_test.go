package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMusicGenerateKeepsLongCallAndCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
		io.WriteString(w, `{"jsonrpc":"2.0","id":"test","result":{"content":[]}}`)
	}))
	defer upstream.Close()
	g := NewGatewayWithClient(nil)
	g.httpClient.Timeout = 20 * time.Millisecond
	g.services["music"] = ServiceRegistration{Endpoint: upstream.URL, Tools: []ToolDefinition{{Name: "music.generate"}, {Name: "music.redraw"}}}
	if _, status, err := g.call(t.Context(), "music.generate", nil, nil); err != nil || status != 200 {
		t.Fatalf("long generation: %d %v", status, err)
	}
	if _, status, err := g.call(t.Context(), "music.redraw", nil, nil); err == nil || status != 502 {
		t.Fatal("unrelated timeout was relaxed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := g.call(ctx, "music.generate", nil, nil); err == nil {
		t.Fatal("cancellation ignored")
	}
	if g.httpClient.Timeout != 20*time.Millisecond {
		t.Fatal("shared client modified")
	}
}

func TestMusicResultLinks(t *testing.T) {
	path := "/artifacts/art_" + strings.Repeat("a", 32) + "/content"
	text := `{"download_path":"` + path + `","size":9007199254740993}`
	raw, _ := json.Marshal(map[string]any{"structuredContent": json.RawMessage(text), "content": []any{map[string]any{"type": "text", "text": text}, map[string]any{"type": "resource_link", "uri": "https://studio.example" + path}}})
	out := musicResult(raw)
	if strings.Count(string(out), musicContentPrefix) != 3 || !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("links/integrity metadata: %s", out)
	}
	if string(musicResult(out)) != string(out) {
		t.Fatal("rewrite is not idempotent")
	}
}

func TestMusicContentTransfer(t *testing.T) {
	t.Setenv("STUDIO_MUSIC_TOKEN", "internal-music")
	t.Setenv("MUSIC_MCP_BEARER_TOKEN", "")
	t.Setenv("INTERNAL_SERVICE_TOKEN", "unrelated")
	path := "/artifacts/art_" + strings.Repeat("b", 32) + "/content"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || r.Header.Get("Authorization") != "Bearer internal-music" || r.Header.Get("Cookie") != "" {
			t.Error("incorrect route/credential")
		}
		http.ServeContent(w, r, "song.wav", time.Time{}, strings.NewReader("music-bytes"))
	}))
	defer upstream.Close()
	g := NewGatewayWithClient(nil)
	g.services["music"] = ServiceRegistration{Endpoint: upstream.URL + "/mcp", Tools: []ToolDefinition{{Name: "music.generate"}}}
	g.services["video"] = ServiceRegistration{Endpoint: "http://127.0.0.1:1/mcp", Tools: []ToolDefinition{{Name: "video.result"}}}
	for _, tc := range []struct {
		rangeHeader, want string
		status            int
	}{{"", "music-bytes", 200}, {"bytes=0-4", "music", 206}, {"bytes=999-", "", 416}} {
		r, status, err := g.MusicContent(t.Context(), musicContentPrefix+path, tc.rangeHeader, map[string]string{"Cookie": "private", "Authorization": "Bearer external"})
		if err != nil || status != tc.status {
			t.Fatalf("download: %d %v", status, err)
		}
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if status != 416 && string(body) != tc.want {
			t.Fatal("incorrect bytes")
		}
	}
	for _, bad := range []string{path, musicContentPrefix + path + "?url=http://other", musicContentPrefix + "/../" + path} {
		if _, status, _ := g.MusicContent(t.Context(), bad, "", nil); status != 400 {
			t.Fatal("invalid path accepted")
		}
	}
	t.Setenv("STUDIO_MUSIC_TOKEN", "")
	if _, status, _ := g.MusicContent(t.Context(), musicContentPrefix+path, "", nil); status != 503 {
		t.Fatal("credential fallback")
	}
	t.Setenv("STUDIO_MUSIC_TOKEN", "internal-music")
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, upstream.URL+path, 307) }))
	defer redirect.Close()
	svc := g.services["music"]
	svc.Endpoint = redirect.URL + "/mcp"
	g.services["music"] = svc
	if _, status, _ := g.MusicContent(t.Context(), musicContentPrefix+path, "", nil); status != 502 {
		t.Fatal("redirect accepted")
	}
}
