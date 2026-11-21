package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestGoogleFlowDesktopRoutesAndWebSocket(t *testing.T) {
	prevInterval := googleFlowRecheckInterval
	googleFlowRecheckInterval = 50 * time.Millisecond
	t.Cleanup(func() { googleFlowRecheckInterval = prevInterval })

	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org := id(), id()
	var open atomic.Bool
	open.Store(true)
	var turnUpstreamStatus atomic.Int32
	turnUpstreamStatus.Store(http.StatusOK)

	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer real-session" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"station_id": id(), "session_id": id(), "user_id": user,
			"organization_id": org, "organization_name": "fixture",
			"request_id": "fixture", "username": "fixture",
			"roles": []string{"admin"}, "scopes": []string{"identity:read"},
			"issued_at": time.Now().Add(-time.Minute).UTC(),
			"expires_at": time.Now().Add(time.Hour).UTC(),
			"revocation_version": 0, "policy_version": 1,
		})
	}))
	defer core.Close()

	disconnected := make(chan struct{}, 2)
	webrtc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("desktop upstream leaked auth/cookie headers")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><head><title>KasmVNC</title></head><body>ok</body>`))
			return
		}
		if r.URL.Path != "/ws" && r.URL.Path != "/websockify" {
			t.Errorf("desktop upstream unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		kind, msg, err := conn.Read(r.Context())
		if err != nil || string(msg) != "HELLO 1" {
			return
		}
		_ = conn.Write(r.Context(), kind, []byte("SESSION_OK"))
		_, _, _ = conn.Read(r.Context())
		disconnected <- struct{}{}
	}))
	defer webrtc.Close()

	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-gflow-token-12345678901234567890" || r.Header.Get("X-Station-Id") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/desktop":
			_ = json.NewEncoder(w).Encode(map[string]any{"available": true, "open": open.Load(), "viewer_path": "/apps/google-flow/desktop/"})
		case r.Method == http.MethodGet && r.URL.Path == "/desktop/settings":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"BDWIND_FRAMERATE": 30})
		case r.Method == http.MethodPost && r.URL.Path == "/desktop/turn":
			st := int(turnUpstreamStatus.Load())
			w.WriteHeader(st)
			if st == http.StatusOK {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"iceTransportPolicy": "all",
					"iceServers": []map[string]any{
						{"urls": []string{"stun:stun.invalid:3478", "turn:192.168.1.200:3478?transport=udp"}, "username": "u", "credential": "c"},
					},
				})
			}
		case r.Method == http.MethodGet && r.URL.Path == "/desktop/assets/index.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<!doctype html><head><script src="studio-embed.js"></script></head><body>ok</body>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mgmt.Close()

	t.Setenv("STUDIO_GOOGLE_FLOW_ORIGIN", mgmt.URL)
	t.Setenv("STUDIO_GOOGLE_FLOW_WEBRTC_ORIGIN", webrtc.URL)
	t.Setenv("STUDIO_APP_GOOGLE_FLOW_TOKEN", "test-gflow-token-12345678901234567890")

	station, err := application.NewStation(core.URL)
	if err != nil {
		t.Fatal(err)
	}
	router, s := NewServer(station, "http://fixture", fstest.MapFS{})
	server := httptest.NewServer(router)
	defer server.Close()
	s.origin = server.URL

	doAuthGet := func(path string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
		req.Header.Set("Authorization", "Bearer real-session")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// 1. Unauthenticated request is rejected.
	resp, err := http.Get(server.URL + "/apps/google-flow/desktop/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 401/403, got %d", resp.StatusCode)
	}

	// 2. Authenticated desktop index proxies KasmVNC HTTP frontend.
	resp = doAuthGet("/apps/google-flow/desktop/")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "KasmVNC") {
		t.Fatalf("expected 200 proxied KasmVNC index, got %d body=%s", resp.StatusCode, string(body))
	}

	// 3. /settings proxies settings JSON when desktop is open.
	resp = doAuthGet("/apps/google-flow/desktop/settings")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "BDWIND_FRAMERATE") {
		t.Fatalf("expected 200 settings, got %d body=%s", resp.StatusCode, string(body))
	}

	// 4. /turn strips non-TURN URLs and forces relay policy; non-200 upstream maps to 502.
	resp = doAuthGet("/apps/google-flow/desktop/turn")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body), "stun:") || !strings.Contains(string(body), `"iceTransportPolicy":"relay"`) {
		t.Fatalf("expected filtered relay TURN JSON, got %d body=%s", resp.StatusCode, string(body))
	}
	turnUpstreamStatus.Store(http.StatusInternalServerError)
	resp = doAuthGet("/apps/google-flow/desktop/turn")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("expected 502 on upstream turn error, got %d", resp.StatusCode)
	}
	turnUpstreamStatus.Store(http.StatusOK)

	// 5. Path traversal and encoded paths return 404.
	resp = doAuthGet("/apps/google-flow/desktop/..%2fsecret")
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for encoded traversal, got %d", resp.StatusCode)
	}

	// 6. Signalling without WebSocket Upgrade header returns 400.
	resp = doAuthGet("/apps/google-flow/desktop/webrtc/signalling/")
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 without upgrade header, got %d", resp.StatusCode)
	}

	// 7. Live WebSocket forwards messages and disconnects when desktop closes during recheck.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/apps/google-flow/desktop/webrtc/signalling/"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer real-session"}},
	})
	if err != nil {
		t.Fatalf("ws dial failed: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, []byte("HELLO 1")); err != nil {
		t.Fatalf("ws write failed: %v", err)
	}
	_, reply, err := conn.Read(ctx)
	if err != nil || string(reply) != "SESSION_OK" {
		t.Fatalf("expected SESSION_OK, got %q err=%v", string(reply), err)
	}
	open.Store(false)
	select {
	case <-disconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("expected live websocket to close after desktop closed on recheck")
	}
	conn.CloseNow()

	// 8. Closed desktop denies /turn and WebSocket handshake with 403.
	resp = doAuthGet("/apps/google-flow/desktop/turn")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for /turn when closed, got %d", resp.StatusCode)
	}
	_, wsResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": []string{"Bearer real-session"}},
	})
	if err == nil || wsResp == nil || wsResp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for ws dial when closed, got resp=%v err=%v", wsResp, err)
	}

	// 9. Missing STUDIO_APP_GOOGLE_FLOW_TOKEN fails closed (403).
	open.Store(true)
	t.Setenv("STUDIO_APP_GOOGLE_FLOW_TOKEN", "")
	resp = doAuthGet("/apps/google-flow/desktop/turn")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when token missing, got %d", resp.StatusCode)
	}
}
