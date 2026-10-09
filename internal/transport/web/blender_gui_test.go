package web

import (
	"context"
	"encoding/json"
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

func TestBlenderGUIRevocationClosesLiveWebSocket(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org := id(), id()
	var revoked atomic.Bool
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked.Load() || r.Header.Get("Authorization") != "Bearer real-session" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"station_id": id(), "session_id": id(), "user_id": user, "organization_id": org, "organization_name": "fixture", "request_id": "fixture", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1})
	}))
	defer core.Close()
	disconnected := make(chan struct{}, 1)
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("worker headers/path invalid")
			w.WriteHeader(400)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_ = conn.Write(r.Context(), websocket.MessageText, []byte("ready"))
		_, _, _ = conn.Read(r.Context())
		disconnected <- struct{}{}
	}))
	defer worker.Close()
	var checks atomic.Int32
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer internal-only" || r.Header.Get("X-User-Id") != user || r.Header.Get("X-Organization-Id") != org {
			t.Error("app trusted context missing")
			w.WriteHeader(403)
			return
		}
		checks.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"gui_origin": worker.URL})
	}))
	defer app.Close()
	station, err := application.NewStation(core.URL)
	if err != nil {
		t.Fatal(err)
	}
	router, s := NewServer(station, "http://fixture", fstest.MapFS{})
	s.blenderEndpoint = func() (string, bool) { return app.URL + "/internal/mcp", true }
	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "internal-only")
	server := httptest.NewServer(router)
	defer server.Close()
	s.origin = server.URL
	base := "/apps/blender/blenderA/" + strings.Repeat("s", 43)
	r := httptest.NewRequest("GET", base+"/settings?save=malicious", nil)
	r.Header.Set("Authorization", "Bearer real-session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 404 || checks.Load() != 0 {
		t.Fatal("settings mutation accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+base+"/webrtc/signalling/", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": []string{"Bearer real-session"}, "Origin": []string{server.URL}, "Cookie": []string{"external=private"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	_, data, err := conn.Read(ctx)
	if err != nil || string(data) != "ready" {
		t.Fatal("WebSocket bytes not proxied", err)
	}
	revoked.Store(true)
	_, _, err = conn.Read(ctx)
	if err == nil || ctx.Err() != nil {
		t.Fatal("revocation did not close active client")
	}
	select {
	case <-disconnected:
	case <-ctx.Done():
		t.Fatal("upstream remained connected")
	}
}
