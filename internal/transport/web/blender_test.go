package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestBlenderProxyIdentityIsolationAndProtocol(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org := id(), id()
	revoked := false
	identity := map[string]any{"station_id": id(), "session_id": id(), "user_id": user, "organization_id": org, "organization_name": "fixture", "request_id": "fixture", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked || r.URL.Path != "/identity/me" || r.Header.Get("Authorization") != "Bearer real-session" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(identity)
	}))
	defer core.Close()
	var received []*http.Request
	var bodies []string
	redirect := false
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = append(received, r.Clone(r.Context()))
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if redirect {
			w.Header().Set("Location", core.URL)
			w.WriteHeader(302)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "internal-secret=do-not-forward")
		if strings.HasPrefix(r.URL.Path, "/internal/instance-") || r.URL.Path == "/internal/instances" || r.URL.Path == "/internal/instances/blenderA/start" || r.URL.Path == "/internal/instances/blenderA/stop" || r.URL.Path == "/internal/instances/blenderA/destroy" {
			if r.Method == "POST" {
				w.WriteHeader(202)
			}
			_, _ = w.Write([]byte(`{"phase":"accepted"}`))
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			_, _ = w.Write([]byte(`{"code":"POST_ONLY"}`))
			return
		}
		if r.URL.Path == "/internal/mcp/blenderB" {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"code":"INSTANCE_NOT_FOUND"}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":9007199254740993,"result":{"tools":[]}}`))
	}))
	defer worker.Close()
	station, err := application.NewStation(core.URL)
	if err != nil {
		t.Fatal(err)
	}
	router, s := NewServer(station, "https://studio.example", fstest.MapFS{})
	s.blenderEndpoint = func() (string, bool) { return worker.URL + "/internal/mcp", true }
	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "internal-only")
	t.Setenv("STUDIO_BEARER_TOKEN", "old-global")
	call := func(method, path, token, body string, extra map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		for k, v := range extra {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	body := `{"jsonrpc":"2.0","id":9007199254740993,"method":"tools/list"}`
	for _, token := range []string{"wrong", "old-global"} {
		w := call("POST", "/mcp/blenderA", token, body, nil)
		if w.Code != 401 && w.Code != 403 {
			t.Fatal("untrusted token accepted", w.Code)
		}
	}
	if len(received) != 0 {
		t.Fatal("unauthorized request forwarded")
	}
	w := call("POST", "/mcp/blenderA", "real-session", body, map[string]string{"X-Instance-Id": "B", "Cookie": "external=secret", "X-Forwarded-Host": "evil.example"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "9007199254740993") {
		t.Fatal(w.Code, w.Body.String())
	}
	if bodies[0] != body {
		t.Fatal("JSON rewritten")
	}
	r := received[0]
	if r.Header.Get("Authorization") != "Bearer internal-only" || r.Header.Get("X-User-Id") != user || r.Header.Get("X-Organization-Id") != org {
		t.Fatal("trusted context missing")
	}
	for _, key := range []string{"Cookie", "X-Instance-Id", "X-Forwarded-Host"} {
		if r.Header.Get(key) != "" {
			t.Fatal("external header leaked", key)
		}
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("internal cookie leaked")
	}
	if call("POST", "/mcp/blenderB", "real-session", body, nil).Code != 404 {
		t.Fatal("instance refusal not preserved")
	}
	if call("GET", "/mcp/blenderA", "real-session", "", nil).Code != 405 {
		t.Fatal("method capability not preserved")
	}
	operation := id()
	for _, tc := range []struct {
		method, path, target, body string
		status                     int
	}{
		{"POST", "/studio/apps/blender/instances", "/internal/instances", `{"name":"Create"}`, 202},
		{"POST", "/studio/apps/blender/instance-projects", "/internal/instance-projects", `{}`, 202},
		{"POST", "/studio/apps/blender/instance-source", "/internal/instance-source", `{}`, 202},
		{"POST", "/studio/apps/blender/instance-project-create", "/internal/instance-project-create", `{}`, 202},
		{"POST", "/studio/apps/blender/instances/blenderA/start", "/internal/instances/blenderA/start", `{"expected_version":2,"idempotency_key":"start-key"}`, 202},
		{"POST", "/studio/apps/blender/instances/blenderA/stop", "/internal/instances/blenderA/stop", `{"expected_version":2,"idempotency_key":"stop-key"}`, 202},
		{"POST", "/studio/apps/blender/instances/blenderA/destroy", "/internal/instances/blenderA/destroy", `{"expected_version":2,"idempotency_key":"destroy-key","confirm_name":"Blender A"}`, 202},
		{"GET", "/studio/apps/blender/instance-options", "/internal/instance-options", "", 200},
		{"GET", "/studio/apps/blender/instance-operations/" + operation, "/internal/instance-operations/" + operation, "", 200},
	} {
		w := call(tc.method, tc.path, "real-session", tc.body, nil)
		if w.Code != tc.status || received[len(received)-1].URL.Path != tc.target || bodies[len(bodies)-1] != tc.body {
			t.Fatal("management forwarding failed", tc.path, w.Code)
		}
	}
	count := len(received)
	for _, tc := range []struct {
		method, path, body string
		extra              map[string]string
		status             int
	}{
		{"POST", "/studio/apps/blender/instances?target=other", `{}`, nil, 400},
		{"POST", "/studio/apps/blender/instances", `{}`, map[string]string{"Content-Type": "text/plain"}, 415},
		{"POST", "/studio/apps/blender/instances", strings.Repeat("x", 16385), nil, 413},
		{"GET", "/studio/apps/blender/instance-operations/invalid", "", nil, 404},
		{"POST", "/studio/apps/blender/instances/invalid/start", `{}`, nil, 404},
		{"POST", "/studio/apps/blender/instances/blenderA/start?target=other", `{}`, nil, 400},
		{"POST", "/studio/apps/blender/instances/blenderA/start", strings.Repeat("x", 16385), nil, 413},
		{"POST", "/studio/apps/blender/instances/invalid/stop", `{}`, nil, 404},
		{"POST", "/studio/apps/blender/instances/blenderA/stop?target=other", `{}`, nil, 400},
		{"POST", "/studio/apps/blender/instances/blenderA/stop", strings.Repeat("x", 16385), nil, 413},
		{"POST", "/studio/apps/blender/instances/invalid/destroy", `{}`, nil, 404},
		{"POST", "/studio/apps/blender/instances/blenderA/destroy?target=other", `{}`, nil, 400},
		{"POST", "/studio/apps/blender/instances/blenderA/destroy", strings.Repeat("x", 16385), nil, 413},
	} {
		if w := call(tc.method, tc.path, "real-session", tc.body, tc.extra); w.Code != tc.status || len(received) != count {
			t.Fatal("invalid management request forwarded", tc.path, w.Code)
		}
	}
	before := len(received)
	if call("POST", "/mcp/blenderA", "real-session", body, map[string]string{"X-User-Id": id()}).Code != 403 {
		t.Fatal("spoofed identity accepted")
	}
	if call("POST", "/mcp/blenderA?target=other", "real-session", body, nil).Code != 404 {
		t.Fatal("query override accepted")
	}
	revoked = true
	if call("POST", "/studio/apps/blender/instances", "real-session", `{}`, nil).Code != 401 {
		t.Fatal("management ignored revocation")
	}
	if call("POST", "/mcp/blenderA", "real-session", body, nil).Code != 401 {
		t.Fatal("revocation ignored")
	}
	if len(received) != before {
		t.Fatal("denied request reached backend")
	}
	revoked = false
	redirect = true
	if call("POST", "/mcp/blenderA", "real-session", body, nil).Code != 502 {
		t.Fatal("redirect followed")
	}
}
