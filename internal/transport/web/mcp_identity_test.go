package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

func TestMCPBearerIdentityAndSessionBoundary(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	user, org := id(), id()
	context := map[string]any{"station_id": id(), "session_id": id(), "user_id": user, "organization_id": org, "organization_name": "fixture", "request_id": "fixture-request", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	revoked := false
	calls := 0
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/identity/me" || r.Header.Get("Authorization") != "Bearer actual-core-session" || revoked {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(context)
	}))
	defer core.Close()
	station, e := application.NewStation(core.URL)
	if e != nil {
		t.Fatal(e)
	}
	router, server := NewServer(station, "https://studio.example", fstest.MapFS{})
	server.SetMCPGateway(mcp.NewGatewayWithClient(nil))
	server.saveSession(t.Context(), "test-cookie-session", "actual-core-session")
	call := func(auth, origin, body string, headers map[string]string, cookie bool) int {
		t.Helper()
		req := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if cookie {
			req.AddCookie(&http.Cookie{Name: cookieName, Value: "test-cookie-session"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	t.Setenv("STUDIO_BEARER_TOKEN", "")
	if code := call("Bearer arbitrary", "", list, nil, false); code != 401 {
		t.Fatal("unconfigured bearer accepted", code)
	}
	if code := call("Bearer actual-core-session", "", list, nil, false); code != 200 {
		t.Fatal(code)
	}
	if code := call("Bearer actual-core-session", "", list, map[string]string{"X-User-Id": id()}, false); code != 403 {
		t.Fatal("spoofed identity accepted", code)
	}
	if code := call("Bearer actual-core-session", "", list, map[string]string{"X-Organization-Id": id()}, false); code != 403 {
		t.Fatal("spoofed organization accepted", code)
	}
	if code := call("", "https://studio.example", list, nil, true); code != 200 {
		t.Fatal("cookie session failed", code)
	}
	if code := call("", "https://attacker.example", list, nil, true); code != 403 {
		t.Fatal("cross-origin cookie accepted", code)
	}
	if code := call("Bearer wrong", "https://studio.example", list, nil, true); code != 401 {
		t.Fatal("failed bearer fell back to cookie", code)
	}
	revoked = true
	if code := call("", "https://studio.example", list, nil, true); code != 401 {
		t.Fatal("cached cookie bypassed revocation", code)
	}
	revoked = false
	context["expires_at"] = time.Now().Add(-time.Second).UTC()
	if code := call("Bearer actual-core-session", "", list, nil, false); code != 401 {
		t.Fatal("expired identity accepted", code)
	}
	context["expires_at"] = time.Now().Add(time.Hour).UTC()
	// The deployment variable must not shadow a valid user bearer or bypass
	// Core. Matching bytes have the same identity rules as any other token.
	t.Setenv("STUDIO_BEARER_TOKEN", "actual-core-session")
	before := calls
	if code := call("Bearer actual-core-session", "", list, nil, false); code != 200 || calls != before+1 {
		t.Fatal("configured bearer did not resolve through Core", code)
	}
	revoked = true
	if code := call("Bearer actual-core-session", "", list, nil, false); code != 401 {
		t.Fatal("revoked configured bearer bypassed Core", code)
	}
	revoked = false
	t.Setenv("STUDIO_BEARER_TOKEN", "unbound-token")
	if code := call("Bearer unbound-token", "", list, nil, false); code != 401 {
		t.Fatal("unbound configured token accepted", code)
	}
	core.Close()
	if code := call("Bearer actual-core-session", "", list, nil, false); code != 503 {
		t.Fatal("Core outage accepted", code)
	}
}
