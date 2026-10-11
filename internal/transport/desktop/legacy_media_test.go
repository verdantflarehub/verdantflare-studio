package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

type memoryLegacySession struct {
	active   bool
	opened   appcenter.LegacyOpened
	requests [][]byte
	forwards int
	read     func(context.Context, string, string, string, []byte, string, string) (*http.Response, error)
}

func (s *memoryLegacySession) Active() bool { return s.active }
func (s *memoryLegacySession) Send(ctx context.Context, method, path string, body []byte) (studioremote.Response, error) {
	if !s.active {
		return studioremote.Response{}, studioremote.ErrChanged
	}
	s.requests = append(s.requests, append([]byte(nil), body...))
	data, _ := json.Marshal(s.opened)
	return studioremote.Response{Status: 200, Data: data, MIME: "application/json"}, nil
}
func (s *memoryLegacySession) LegacyMedia(ctx context.Context, adapter, method, path string, body []byte, kind, byteRange string) (*http.Response, error) {
	s.forwards++
	return s.read(ctx, adapter, method, path, body, kind, byteRange)
}

func legacyDesktopFixture(t *testing.T) (*desktopLegacyMedia, *memoryLegacySession, []byte) {
	t.Helper()
	digest := "sha256:" + strings.Repeat("a", 64)
	s := &memoryLegacySession{active: true, opened: appcenter.LegacyOpened{SchemaVersion: 1, Transport: "legacy", AdapterID: "video-iframe-v1", BindingID: "old-video", BindingRevision: digest, Route: "/market?video=%2Fdashboard%23tasks"}}
	s.read = func(context.Context, string, string, string, []byte, string, string) (*http.Response, error) {
		data := `{"tasks":[]}`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(data))}, nil
	}
	b := &desktopLegacyMedia{leases: map[string]*legacyMediaLease{}, now: time.Now, bind: func() (legacyMediaSession, error) { return s, nil }}
	body, _ := json.Marshal(appcenter.OpenRequest{SchemaVersion: 1, CardKey: "obs:" + strings.Repeat("b", 64), EntryID: "video-iframe-v1", ExpectedViewRevision: digest, ExpectedBindingRevision: digest})
	t.Cleanup(func() {
		for id := range b.leases {
			b.remove(id)
		}
	})
	return b, s, body
}

func desktopLegacyRequest(b *desktopLegacyMedia, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://wails.localhost"+path, bytes.NewReader(body))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}
func openDesktopLegacy(t *testing.T, b *desktopLegacyMedia, body []byte) string {
	t.Helper()
	w := desktopLegacyRequest(b, "POST", legacyWorkspacePrefix, body, nil)
	if w.Code != 200 {
		t.Fatalf("open: %d %s", w.Code, w.Body.String())
	}
	id := w.Header().Get("X-Studio-Legacy-Workspace")
	if id == "" {
		t.Fatal("missing handle")
	}
	return legacyWorkspacePrefix + "/" + id
}

func TestDesktopLegacyMediaBoundOpenReadAndClose(t *testing.T) {
	b, s, body := legacyDesktopFixture(t)
	prefix := openDesktopLegacy(t, b, body)
	w := desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil)
	if w.Code != 200 || w.Body.String() != `{"tasks":[]}` || s.forwards != 1 {
		t.Fatal("read failed")
	}
	if len(s.requests) != 2 || !bytes.Equal(s.requests[0], s.requests[1]) {
		t.Fatal("resource request did not recheck original binding")
	}
	lease := b.leases[strings.TrimPrefix(prefix, legacyWorkspacePrefix+"/")]
	w = desktopLegacyRequest(b, "DELETE", prefix, nil, nil)
	if w.Code != 204 || lease.ctx.Err() == nil || len(b.leases) != 0 {
		t.Fatal("workspace not released")
	}
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 410 || s.forwards != 1 {
		t.Fatal("closed workspace forwarded")
	}
}

func TestDesktopLegacyMediaCSPAndOpaqueFrameResources(t *testing.T) {
	b, s, body := legacyDesktopFixture(t)
	prefix := openDesktopLegacy(t, b, body)
	s.read = func(_ context.Context, _, _, path string, _ []byte, _, _ string) (*http.Response, error) {
		data := `<!doctype html><link href="./dashboard/frontend/dashboard.css" rel="stylesheet"><script src="./dashboard/frontend/studio-embed.js"></script><a href="/dashboard">back</a>`
		kind := "text/html"
		if strings.HasSuffix(path, ".js") {
			data = "window.ready=true"
			kind = "text/javascript"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {kind}}, ContentLength: int64(len(data)), Body: io.NopCloser(strings.NewReader(data))}, nil
	}
	w := desktopLegacyRequest(b, "GET", prefix+"/dashboard?embed=1&theme=light&view=1", nil, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), prefix+"/dashboard/frontend/studio-embed.js") || !strings.Contains(w.Body.String(), `<a href="/dashboard">back</a>`) {
		t.Fatal("HTML resources not bound")
	}
	csp := w.Header().Get("Content-Security-Policy")
	for _, part := range []string{"script-src http://wails.localhost;", "connect-src 'none'", "base-uri 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(csp, part) {
			t.Fatalf("CSP missing %s", part)
		}
	}
	if strings.Contains(csp, "unsafe-inline'; style-src") {
		t.Fatal("video inline script permission expanded")
	}
	opaque := map[string]string{"Origin": "null", "Sec-Fetch-Site": "cross-site"}
	w = desktopLegacyRequest(b, "GET", prefix+"/dashboard/frontend/studio-embed.js", nil, opaque)
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("opaque script blocked")
	}
	before := s.forwards
	w = desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, opaque)
	if w.Code != 403 || s.forwards != before {
		t.Fatal("opaque frame directly read business API")
	}
}

func TestDesktopLegacyMediaBindingAndSessionChanges(t *testing.T) {
	b, s, body := legacyDesktopFixture(t)
	prefix := openDesktopLegacy(t, b, body)
	s.opened.BindingID = "replacement"
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 409 || s.forwards != 0 || len(b.leases) != 0 {
		t.Fatal("changed backend forwarded")
	}
	prefix = openDesktopLegacy(t, b, body)
	s.active = false
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 410 || s.forwards != 0 {
		t.Fatal("old session survived")
	}
	// A new authenticated session must never be acquired by an old handle.
	s.active = true
	prefix = openDesktopLegacy(t, b, body)
	s.active = false
	b.bind = func() (legacyMediaSession, error) { t.Fatal("old handle acquired new session"); return nil, nil }
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 410 {
		t.Fatal("old handle changed login")
	}
}

func TestDesktopLegacyMediaLimitsExpiryAndHostGuards(t *testing.T) {
	b, s, body := legacyDesktopFixture(t)
	now := time.Now()
	b.now = func() time.Time { return now }
	prefix := openDesktopLegacy(t, b, body)
	for _, headers := range []map[string]string{
		{"Origin": "https://foreign.invalid"}, {"Cookie": "browser=credential"}, {"Authorization": "Bearer browser-value"}, {"Content-Encoding": "gzip"}, {"Sec-Fetch-Site": "cross-site"},
	} {
		if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, headers).Code == 200 {
			t.Fatal("untrusted request allowed")
		}
	}
	if s.forwards != 0 {
		t.Fatal("guarded request sent")
	}
	lease := b.leases[strings.TrimPrefix(prefix, legacyWorkspacePrefix+"/")]
	for range 32 {
		lease.active <- struct{}{}
	}
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 429 {
		t.Fatal("concurrency unbounded")
	}
	for range 32 {
		<-lease.active
	}
	for range 15 {
		openDesktopLegacy(t, b, body)
	}
	if desktopLegacyRequest(b, "POST", legacyWorkspacePrefix, body, nil).Code != 429 {
		t.Fatal("workspaces unbounded")
	}
	now = now.Add(31 * time.Minute)
	if desktopLegacyRequest(b, "GET", prefix+"/api/dashboard", nil, nil).Code != 410 || len(b.leases) != 0 || lease.ctx.Err() == nil {
		t.Fatal("idle expiry failed")
	}
	openDesktopLegacy(t, b, body)
	for _, host := range []string{"https://foreign.invalid", "http://wails.localhost@foreign.invalid", "file://wails.localhost"} {
		r := httptest.NewRequest("GET", host+"/", nil)
		if localLegacyOrigin(r) != "" {
			t.Fatal("non-host origin accepted")
		}
	}
}

func TestRewriteLegacyMediaHTMLPreservesBusinessAndForeignReferences(t *testing.T) {
	input := []byte(`<script>const text="./dashboard/frontend/dashboard.js";</script><link href="https://foreign.invalid/dashboard/frontend/dashboard.css"><script src="/apps/image/dashboard/static/dashboard.js"></script><script src="/apps/video/dashboard/frontend/dashboard.js"></script>`)
	out, err := rewriteLegacyMediaHTML(input, "video-iframe-v1", "/bound")
	if err != nil {
		t.Fatal(err)
	}
	for _, same := range []string{`const text="./dashboard/frontend/dashboard.js";`, `href="https://foreign.invalid/dashboard/frontend/dashboard.css"`, `src="/apps/image/dashboard/static/dashboard.js"`} {
		if !strings.Contains(string(out), same) {
			t.Fatal("unrelated markup rewritten")
		}
	}
	if !strings.Contains(string(out), `src="/bound/dashboard/frontend/dashboard.js"`) {
		t.Fatal("registered URL not bound")
	}
}
