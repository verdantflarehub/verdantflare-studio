package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

type memoryBlenderSession struct {
	mu       sync.Mutex
	active   bool
	opened   appcenter.LegacyOpened
	checks   [][]byte
	requests []string
	read     func(context.Context, string, string, []byte) (studioremote.Response, error)
}

func (s *memoryBlenderSession) Active() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.active }
func (s *memoryBlenderSession) Send(_ context.Context, method, path string, body []byte) (studioremote.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks = append(s.checks, append([]byte(nil), body...))
	data, _ := json.Marshal(s.opened)
	if !s.active {
		return studioremote.Response{}, studioremote.ErrChanged
	}
	return studioremote.Response{Status: 200, MIME: "application/json", Data: data}, nil
}
func (s *memoryBlenderSession) LegacyBlender(ctx context.Context, method, path string, body []byte) (studioremote.Response, error) {
	s.mu.Lock()
	s.requests = append(s.requests, method+" "+path+" "+string(body))
	read := s.read
	s.mu.Unlock()
	if read != nil {
		return read(ctx, method, path, body)
	}
	data := `{"instances":[]}`
	if strings.HasSuffix(path, "/open") {
		data = `{"editing_session_id":"` + strings.Repeat("s", 43) + `","reconnect_after":720}`
	}
	return studioremote.Response{Status: 200, MIME: "application/json", Data: []byte(data)}, nil
}

type memoryBlenderWire struct {
	incoming chan []byte
	closed   chan struct{}
	once     sync.Once
	mu       sync.Mutex
	writes   []string
}

func (w *memoryBlenderWire) Read(ctx context.Context) ([]byte, error) {
	select {
	case data := <-w.incoming:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-w.closed:
		return nil, errors.New("closed")
	}
}
func (w *memoryBlenderWire) Write(ctx context.Context, data []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.closed:
		return errors.New("closed")
	default:
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes = append(w.writes, string(data))
	return nil
}
func (w *memoryBlenderWire) Close() { w.once.Do(func() { close(w.closed) }) }

func blenderDesktopFixture(t *testing.T) (*desktopLegacyBlender, *memoryBlenderSession, []byte, **memoryBlenderWire) {
	t.Helper()
	digest := "sha256:" + strings.Repeat("a", 64)
	s := &memoryBlenderSession{active: true, opened: appcenter.LegacyOpened{SchemaVersion: 1, Transport: "legacy", AdapterID: "blender-host-v1", BindingID: "original-blender", BindingRevision: digest, Route: "/market?blender=instances"}}
	b := &desktopLegacyBlender{leases: map[string]*desktopBlenderLease{}, now: time.Now, checkInterval: time.Hour, pollTimeout: 10 * time.Millisecond, bind: func() (blenderRemoteSession, error) { return s, nil }}
	var wire *memoryBlenderWire
	b.dial = func(ctx context.Context, _ blenderRemoteSession, alias, id string) (blenderWire, error) {
		wire = &memoryBlenderWire{incoming: make(chan []byte, 40), closed: make(chan struct{})}
		return wire, nil
	}
	body, _ := json.Marshal(appcenter.OpenRequest{SchemaVersion: 1, CardKey: "obs:" + strings.Repeat("b", 64), EntryID: "blender-host-v1", ExpectedViewRevision: digest, ExpectedBindingRevision: digest})
	t.Cleanup(func() {
		b.mu.Lock()
		ids := []string{}
		for id := range b.leases {
			ids = append(ids, id)
		}
		b.mu.Unlock()
		for _, id := range ids {
			b.remove(id)
		}
	})
	return b, s, body, &wire
}
func callBlender(b *desktopLegacyBlender, method, path string, body []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://wails.localhost"+path, bytes.NewReader(body))
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}
func openBlenderWorkspace(t *testing.T, b *desktopLegacyBlender, body []byte) string {
	t.Helper()
	w := callBlender(b, "POST", blenderWorkspacePrefix, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	return blenderWorkspacePrefix + "/" + w.Header().Get("X-Studio-Legacy-Workspace")
}
func blenderEnvelope(method, path, body string) []byte {
	data, _ := json.Marshal(map[string]string{"method": method, "path": path, "body": body})
	return data
}
func openBlenderEdit(t *testing.T, b *desktopLegacyBlender, prefix string) {
	t.Helper()
	w := callBlender(b, "POST", prefix+"/requests", blenderEnvelope("POST", "/studio/apps/blender/blenderA/open", `{}`))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
}
func openBlenderSignal(t *testing.T, b *desktopLegacyBlender, prefix string) string {
	t.Helper()
	data, _ := json.Marshal(map[string]string{"alias": "blenderA", "editingSession": strings.Repeat("s", 43)})
	w := callBlender(b, "POST", prefix+"/signals", data)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var out struct {
		ID string `json:"signalId"`
	}
	if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.ID == "" {
		t.Fatal("no signal")
	}
	return prefix + "/signals/" + out.ID
}
func waitBlenderClosed(t *testing.T, w *memoryBlenderWire) {
	t.Helper()
	select {
	case <-w.closed:
	case <-time.After(time.Second):
		t.Fatal("signal not closed")
	}
}

func TestDesktopLegacyBlenderBindingAndOriginalRequest(t *testing.T) {
	b, s, body, _ := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	original := " \n{\"idempotency_key\":\"original\",\"expected_version\":2} \n"
	w := callBlender(b, "POST", prefix+"/requests", blenderEnvelope("POST", "/studio/apps/blender/instances/blenderA/start", original))
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.checks) != 2 || !bytes.Equal(s.checks[0], body) || !bytes.Equal(s.checks[1], body) || len(s.requests) != 1 || !strings.HasSuffix(s.requests[0], original) {
		t.Fatal("original binding/body lost")
	}
}

func TestDesktopLegacyBlenderSignalRoundTripAndGuards(t *testing.T) {
	b, _, body, wire := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	other := openBlenderWorkspace(t, b, body)
	unbound := []byte(`{"alias":"blenderA","editingSession":"` + strings.Repeat("s", 43) + `"}`)
	if callBlender(b, "POST", prefix+"/signals", unbound).Code != 409 {
		t.Fatal("unbound edit accepted")
	}
	openBlenderEdit(t, b, prefix)
	signal := openBlenderSignal(t, b, prefix)
	if callBlender(b, "POST", prefix+"/signals", unbound).Code != 409 || callBlender(b, "POST", other+"/signals", unbound).Code != 409 {
		t.Fatal("duplicate/foreign signal accepted")
	}
	(*wire).incoming <- []byte("HELLO original")
	w := callBlender(b, "GET", signal, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "HELLO original") {
		t.Fatal("read failed", w.Code)
	}
	if callBlender(b, "POST", signal, []byte(`{"message":"SESSION original"}`)).Code != 204 {
		t.Fatal("write failed")
	}
	(*wire).mu.Lock()
	got := strings.Join((*wire).writes, ",")
	(*wire).mu.Unlock()
	if got != "SESSION original" {
		t.Fatal("signal changed")
	}
	if callBlender(b, "GET", signal, nil).Code != 204 {
		t.Fatal("empty poll failed")
	}
	foreign := other + strings.TrimPrefix(signal, prefix)
	if callBlender(b, "GET", foreign, nil).Code != 410 {
		t.Fatal("foreign read accepted")
	}
	closeBody := blenderEnvelope("POST", "/studio/apps/blender/blenderA/close", `{"editing_session_id":"`+strings.Repeat("s", 43)+`"}`)
	if callBlender(b, "POST", other+"/requests", closeBody).Code != 403 {
		t.Fatal("foreign close accepted")
	}
	if callBlender(b, "POST", prefix+"/requests", closeBody).Code != 200 {
		t.Fatal("own close failed")
	}
	waitBlenderClosed(t, *wire)
	if callBlender(b, "GET", signal, nil).Code != 410 {
		t.Fatal("closed edit signal usable")
	}
}

func TestDesktopLegacyBlenderRevocationAndQueueOverflow(t *testing.T) {
	for _, mode := range []string{"delete", "binding", "logout", "overflow", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			b, s, body, wire := blenderDesktopFixture(t)
			prefix := openBlenderWorkspace(t, b, body)
			openBlenderEdit(t, b, prefix)
			signal := openBlenderSignal(t, b, prefix)
			switch mode {
			case "delete":
				if callBlender(b, "DELETE", prefix, nil).Code != 204 {
					t.Fatal("delete failed")
				}
			case "binding":
				s.mu.Lock()
				s.opened.BindingID = "replacement"
				s.mu.Unlock()
				if callBlender(b, "GET", signal, nil).Code != 409 {
					t.Fatal("changed entry accepted")
				}
			case "logout":
				s.mu.Lock()
				s.active = false
				s.mu.Unlock()
				if callBlender(b, "GET", signal, nil).Code != 410 {
					t.Fatal("changed login accepted")
				}
			case "overflow":
				for range 33 {
					(*wire).incoming <- []byte("message")
				}
			case "oversize":
				(*wire).incoming <- []byte(strings.Repeat("x", 256<<10+1))
			}
			waitBlenderClosed(t, *wire)
			if w := callBlender(b, "GET", signal, nil); w.Code == 200 {
				t.Fatal("revoked queue returned bytes")
			}
		})
	}
}

func TestDesktopLegacyBlenderLocalOriginAndCapacity(t *testing.T) {
	b, s, body, _ := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	for _, headers := range []map[string]string{{"Cookie": "browser=private"}, {"Authorization": "Bearer private"}, {"Origin": "https://foreign.invalid"}, {"Origin": "null"}, {"Sec-Fetch-Site": "cross-site"}, {"Range": "bytes=0-1"}} {
		r := httptest.NewRequest("POST", "http://wails.localhost"+prefix+"/requests", bytes.NewReader(blenderEnvelope("GET", "/studio/apps/blender/instances", "")))
		r.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		b.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("untrusted host accepted")
		}
	}
	s.mu.Lock()
	n := len(s.requests)
	s.mu.Unlock()
	if n != 0 {
		t.Fatal("guarded request forwarded")
	}
	if callBlender(b, "GET", "/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/", nil).Code != 410 {
		t.Fatal("unbound asset accepted")
	}
	for range 15 {
		openBlenderWorkspace(t, b, body)
	}
	if callBlender(b, "POST", blenderWorkspacePrefix, body).Code != 429 {
		t.Fatal("workspace limit ignored")
	}
	now := time.Now().Add(31 * time.Minute)
	b.now = func() time.Time { return now }
	if callBlender(b, "GET", prefix+"/signals/00000000-0000-4000-8000-000000000000", nil).Code != 410 {
		t.Fatal("expired workspace accepted")
	}
}

func TestDesktopLegacyBlenderWatchdog(t *testing.T) {
	b, s, body, wire := blenderDesktopFixture(t)
	b.checkInterval = 5 * time.Millisecond
	prefix := openBlenderWorkspace(t, b, body)
	openBlenderEdit(t, b, prefix)
	openBlenderSignal(t, b, prefix)
	s.mu.Lock()
	s.opened.BindingID = "replacement"
	s.mu.Unlock()
	waitBlenderClosed(t, *wire)
	if callBlender(b, "POST", prefix+"/requests", blenderEnvelope("GET", "/studio/apps/blender/instances", "")).Code != 410 {
		t.Fatal("watchdog retained workspace")
	}
}

func TestDesktopLegacyBlenderLateOpenAndCleanup(t *testing.T) {
	b, s, body, _ := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	released := make(chan struct{})
	s.read = func(_ context.Context, _, path string, _ []byte) (studioremote.Response, error) {
		if strings.HasSuffix(path, "/open") {
			cancel()
			return studioremote.Response{Status: 200, MIME: "application/json", Data: []byte(`{"editing_session_id":"` + strings.Repeat("s", 43) + `"}`)}, nil
		}
		if strings.HasSuffix(path, "/close") {
			close(released)
			return studioremote.Response{Status: 204}, nil
		}
		return studioremote.Response{}, errors.New("unexpected path")
	}
	r := httptest.NewRequest("POST", "http://wails.localhost"+prefix+"/requests", bytes.NewReader(blenderEnvelope("POST", "/studio/apps/blender/blenderA/open", `{}`))).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	if w.Code != 409 {
		t.Fatal("cancelled open returned editing handle")
	}
	select {
	case <-released:
	case <-time.After(time.Second):
		t.Fatal("late edit not released")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, lease := range b.leases {
		if len(lease.edits) != 0 || lease.opening != 0 {
			t.Fatal("late edit retained")
		}
	}
}

func TestDesktopLegacyBlenderCloseOnlyCapturedEdits(t *testing.T) {
	b, s, body, wire := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	openBlenderEdit(t, b, prefix)
	openBlenderSignal(t, b, prefix)
	released := make(chan string, 1)
	s.mu.Lock()
	s.read = func(_ context.Context, method, path string, body []byte) (studioremote.Response, error) {
		released <- method + " " + path + " " + string(body)
		return studioremote.Response{Status: 204}, nil
	}
	s.mu.Unlock()
	if callBlender(b, "DELETE", prefix, nil).Code != 204 {
		t.Fatal("delete failed")
	}
	waitBlenderClosed(t, *wire)
	select {
	case request := <-released:
		if request != `POST /studio/apps/blender/blenderA/close {"editing_session_id":"`+strings.Repeat("s", 43)+`"}` {
			t.Fatal("cleanup changed target")
		}
	case <-time.After(time.Second):
		t.Fatal("captured edit not released")
	}
}

func TestDesktopLegacyBlenderSignalConcurrencyAndCancellation(t *testing.T) {
	b, _, body, wire := blenderDesktopFixture(t)
	b.pollTimeout = time.Second
	prefix := openBlenderWorkspace(t, b, body)
	openBlenderEdit(t, b, prefix)
	signalPath := openBlenderSignal(t, b, prefix)
	b.mu.Lock()
	var signal *desktopBlenderSignal
	for _, lease := range b.leases {
		for _, edit := range lease.edits {
			signal = edit.signal
		}
	}
	b.mu.Unlock()
	signal.reading <- struct{}{}
	if callBlender(b, "GET", signalPath, nil).Code != 409 {
		t.Fatal("concurrent read accepted")
	}
	<-signal.reading
	signal.writing <- struct{}{}
	if callBlender(b, "POST", signalPath, []byte(`{"message":"hello"}`)).Code != 409 {
		t.Fatal("concurrent write accepted")
	}
	<-signal.writing
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest("GET", "http://wails.localhost"+signalPath, nil).WithContext(ctx)
	done := make(chan int, 1)
	go func() { w := httptest.NewRecorder(); b.ServeHTTP(w, r); done <- w.Code }()
	deadline := time.After(time.Second)
	for len(signal.reading) == 0 {
		select {
		case <-deadline:
			t.Fatal("poll never started")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()
	select {
	case status := <-done:
		if status != 410 {
			t.Fatal(status)
		}
	case <-time.After(time.Second):
		t.Fatal("poll did not cancel")
	}
	waitBlenderClosed(t, *wire)
	if callBlender(b, "POST", prefix+"/requests", blenderEnvelope("GET", "/studio/apps/blender/instances", "")).Code != 200 {
		t.Fatal("signal cancellation removed valid workspace")
	}
}

func TestDesktopLegacyBlenderBoundAssetsAndPendingViewer(t *testing.T) {
	b, s, body, _ := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	openBlenderEdit(t, b, prefix)
	s.mu.Lock()
	s.read = func(_ context.Context, method, path string, body []byte) (studioremote.Response, error) {
		kind := "text/javascript"
		data := "window.ready=true"
		if strings.HasSuffix(path, "/") {
			kind = "text/html"
			data = "<html><head></head></html>"
		}
		return studioremote.Response{Status: 200, MIME: kind, Data: []byte(data)}, nil
	}
	s.mu.Unlock()
	root := "/apps/blender/blenderA/" + strings.Repeat("s", 43) + "/"
	w := callBlender(b, "GET", root+"studio-embed.js", nil)
	if w.Code != 200 || w.Body.String() != "window.ready=true" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("bound asset failed")
	}
	if callBlender(b, "GET", root, nil).Code != 409 {
		t.Fatal("viewer enabled before shim")
	}
	if callBlender(b, "GET", strings.Replace(root, "blenderA", "blenderB", 1)+"studio-embed.js", nil).Code != 410 {
		t.Fatal("asset alias changed")
	}
	if callBlender(b, "DELETE", prefix, nil).Code != 204 || callBlender(b, "GET", root+"studio-embed.js", nil).Code != 410 {
		t.Fatal("closed assets survived")
	}
}

func TestDesktopLegacyBlenderUnavailableRemoteRouting(t *testing.T) {
	fallback := false
	h := RemoteAssetHandler(&Service{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { fallback = true }))
	r := httptest.NewRequest("GET", "http://wails.localhost/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/settings", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if fallback || w.Code != 410 {
		t.Fatal("unbound desktop resource fell through")
	}
}

func TestDesktopLegacyBlenderClosedEditRejectsLateAsset(t *testing.T) {
	b, s, body, _ := blenderDesktopFixture(t)
	prefix := openBlenderWorkspace(t, b, body)
	openBlenderEdit(t, b, prefix)
	started, finish := make(chan struct{}), make(chan struct{})
	s.mu.Lock()
	s.read = func(_ context.Context, method, path string, body []byte) (studioremote.Response, error) {
		if method == "GET" {
			close(started)
			<-finish
			return studioremote.Response{Status: 200, MIME: "text/javascript", Data: []byte("late-private-asset")}, nil
		}
		return studioremote.Response{Status: 204}, nil
	}
	s.mu.Unlock()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- callBlender(b, "GET", "/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/studio-embed.js", nil)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("asset did not start")
	}
	input := blenderEnvelope("POST", "/studio/apps/blender/blenderA/close", `{"editing_session_id":"`+strings.Repeat("s", 43)+`"}`)
	if callBlender(b, "POST", prefix+"/requests", input).Code != 204 {
		t.Fatal("edit close failed")
	}
	close(finish)
	select {
	case out := <-done:
		if out.Code != 409 || strings.Contains(out.Body.String(), "late-private-asset") {
			t.Fatal("closed edit leaked late bytes")
		}
	case <-time.After(time.Second):
		t.Fatal("asset did not finish")
	}
}
