package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

// Opens a test HTTP listener; run in the documented development environment.
func TestDesktopLegacyMediaGatewayHTTP(t *testing.T) {
	_, fixture, openBody := legacyDesktopFixture(t)
	var block atomic.Bool
	started, cancelled := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/api/login" {
			http.SetCookie(w, &http.Cookie{Name: "vf_studio_session", Value: "test-only", Path: "/", HttpOnly: true})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"user_id":"test"}`)
			return
		}
		cookie, err := r.Cookie("vf_studio_session")
		if err != nil || cookie.Value != "test-only" || r.Header.Get("Authorization") != "" {
			t.Error("missing host cookie or leaked auth header")
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/studio/appcenter/v1/open":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(fixture.opened)
		case "/studio/apps/video/dashboard":
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, `<script src="./dashboard/frontend/studio-embed.js"></script>`)
		case "/studio/apps/video/dashboard/frontend/studio-embed.js":
			w.Header().Set("Content-Type", "text/javascript")
			_, _ = io.WriteString(w, "window.ready=true")
		case "/studio/apps/video/artifacts/a/content":
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("ETag", `"fixed-test"`)
			http.ServeContent(w, r, "test.mp4", time.Time{}, strings.NewReader("0123456789"))
		case "/studio/apps/video/api/dashboard":
			if block.Load() {
				close(started)
				select {
				case <-r.Context().Done():
					close(cancelled)
				case <-time.After(5 * time.Second):
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"tasks":[]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	remote, err := studioremote.New(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	service := NewRemote(remote)
	if result := service.Call(context.Background(), application.Request{Method: "POST", Path: "login"}); result.Status != 201 {
		t.Fatal("login failed")
	}
	handler := RemoteAssetHandler(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("legacy path fell back to shell")
		w.WriteHeader(500)
	}))
	request := func(method, path string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://wails.localhost"+path, body)
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	opened := request("POST", legacyWorkspacePrefix, strings.NewReader(string(openBody)), nil)
	if opened.Code != 200 {
		t.Fatalf("open failed: %d %s", opened.Code, opened.Body.String())
	}
	prefix := legacyWorkspacePrefix + "/" + opened.Header().Get("X-Studio-Legacy-Workspace")
	page := request("GET", prefix+"/dashboard?embed=1&theme=light&view=1", nil, nil)
	if page.Code != 200 || !strings.Contains(page.Body.String(), prefix+"/dashboard/frontend/studio-embed.js") {
		t.Fatal("bound HTML missing")
	}
	script := request("GET", prefix+"/dashboard/frontend/studio-embed.js", nil, map[string]string{"Origin": "null", "Sec-Fetch-Site": "cross-site"})
	if script.Code != 200 || script.Body.String() != "window.ready=true" {
		t.Fatal("script transfer failed")
	}
	media := request("GET", prefix+"/artifacts/a/content", nil, map[string]string{"Range": "bytes=2-4"})
	if media.Code != 206 || media.Body.String() != "234" || media.Header().Get("Content-Range") != "bytes 2-4/10" || media.Header().Get("ETag") != `"fixed-test"` {
		t.Fatal("range transfer failed")
	}
	block.Store(true)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- request("GET", prefix+"/api/dashboard", nil, nil) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("media request did not start")
	}
	if closed := request("DELETE", prefix, nil, nil); closed.Code != 204 {
		t.Fatal("close failed")
	}
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not cancel remote HTTP")
	}
	select {
	case result := <-done:
		if result.Code < 400 {
			t.Fatal("cancelled read succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("local request leaked")
	}
	before := calls.Load()
	if result := request("GET", prefix+"/api/dashboard", nil, nil); result.Code != 410 || calls.Load() != before {
		t.Fatal("closed handle sent remote request")
	}
}
