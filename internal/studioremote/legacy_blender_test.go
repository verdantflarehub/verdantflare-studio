package studioremote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func blenderSaveCall(tool string) string {
	return `{"jsonrpc":"2.0","id":"request-original","method":"tools/call","params":{"name":"` + tool + `","arguments":{}}}`
}

func TestLegacyBlenderRoutePolicy(t *testing.T) {
	base := "/studio/apps/blender/"
	for _, name := range []string{"instances", "instance-options", "instance-operations/019a1234-1234-7123-8123-123456789abc"} {
		if _, _, err := legacyBlenderRoute("GET", base+name, nil); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"instances", "instance-projects", "instance-source", "instance-project-create", "instances/blenderA/start", "instances/blenderA/stop", "instances/blenderA/destroy", "blenderA/open", "blenderA/close"} {
		if _, _, err := legacyBlenderRoute("POST", base+name, []byte(" \n{\"idempotency_key\":\"original\"} \n")); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, tool := range []string{"session.open", "scene.get", "project.save", "operation.get", "session.close"} {
		if _, _, err := legacyBlenderRoute("POST", "/mcp/blenderA", []byte(blenderSaveCall(tool))); err != nil {
			t.Fatal(tool, err)
		}
	}
	gui := "/apps/blender/blenderA/" + strings.Repeat("s", 43) + "/"
	for _, name := range []string{"", "index.html", "studio-embed.js", "assets/client.js", "assets/client.css", "assets/logo.svg", "assets/font.woff2", "settings", "turn"} {
		if _, _, err := legacyBlenderRoute("GET", gui+name, nil); err != nil {
			t.Fatal(name, err)
		}
	}
	for _, target := range []string{base + "instances?project=x", base + "instances?", base + "instances#x", base + "%69nstances", base + "instances/", base + "instances/../instances", base + "instances//blenderA", base + "instance-operations/invalid", base + "instance-operations/019a1234-1234-4123-8123-123456789abc", "https://foreign.invalid" + base + "instances", gui + "../index.html", gui + "assets//client.js", gui + "assets/secret.txt", gui + "webrtc/signalling/", gui + "settings?save=true"} {
		if _, _, err := legacyBlenderRoute("GET", target, nil); err == nil {
			t.Fatal("accepted", target)
		}
	}
	for _, tc := range []struct{ method, target, body string }{
		{"GET", base + "instances", `{}`}, {"POST", base + "instance-options", `{}`}, {"DELETE", base + "instances", `{}`},
		{"POST", base + "instances", `{"a":1,"a":2}`}, {"POST", base + "instances", `null`}, {"POST", base + "instances", `[]`}, {"POST", base + "instances", ""},
		{"POST", base + "instances", `{"name":"` + strings.Repeat("x", 16<<10) + `"}`}, {"POST", base + "instances/other/start", `{}`},
		{"POST", "/mcp/blenderA", blenderSaveCall("python.execute")}, {"POST", "/mcp/blenderA", `{"jsonrpc":"2.0","method":"tools/list","id":"x"}`},
		{"POST", "/mcp/blenderA", strings.Replace(blenderSaveCall("scene.get"), `"request-original"`, `null`, 1)},
		{"POST", "/mcp/other", blenderSaveCall("scene.get")}, {"POST", gui + "settings", `{}`},
	} {
		if _, _, err := legacyBlenderRoute(tc.method, tc.target, []byte(tc.body)); err == nil {
			t.Fatal("accepted invalid request", tc.target)
		}
	}
	if Allowed("GET", base+"instances") || Allowed("POST", "/mcp/blenderA") || Allowed("GET", gui+"settings") {
		t.Fatal("public bridge expanded")
	}
}

func TestLegacyBlenderPreservesBytesAndOriginalSession(t *testing.T) {
	original := " \n{\"idempotency_key\":\"same-key\",\"expected_version\":2} \n"
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/studio/apps/blender/instances/blenderA/start" || string(body) != original || r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Error("original request changed")
		}
		out := legacyResponse(202, "application/json", `{"operation_id":"same-operation"}`)
		out.Header.Set("Set-Cookie", "never-expose=private")
		out.Header.Set("Content-Security-Policy", "unsafe-policy")
		return out, nil
	})
	login(t, c)
	s, _ := c.Session()
	out, err := s.LegacyBlender(context.Background(), "POST", "/studio/apps/blender/instances/blenderA/start", []byte(original))
	if err != nil || out.Status != 202 || string(out.Data) != `{"operation_id":"same-operation"}` || out.MIME != "application/json" {
		t.Fatal(out, err)
	}
	if len(c.transfers) != 0 || calls != 1 {
		t.Fatal("transfer leaked or retried")
	}
	login(t, c)
	if _, err = s.LegacyBlender(context.Background(), "POST", "/studio/apps/blender/instances/blenderA/start", []byte(original)); !errors.Is(err, ErrChanged) || calls != 1 {
		t.Fatal("old workspace acquired new login")
	}
}

func TestLegacyBlenderResponseBoundsAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response *http.Response
		want     error
		status   int
	}{
		{"json", legacyResponse(200, "application/json", `{"instances":[]}`), nil, 200},
		{"empty rejection", legacyResponse(403, "", ""), nil, 403},
		{"empty success", legacyResponse(200, "application/json", ""), ErrInvalid, 0},
		{"html rejection", legacyResponse(502, "text/html", "<html>upstream</html>"), ErrInvalid, 0},
		{"duplicate", legacyResponse(200, "application/json", `{"a":1,"a":2}`), ErrInvalid, 0},
		{"redirect", legacyResponse(302, "application/json", `{}`), ErrDenied, 0},
		{"partial", legacyResponse(206, "application/json", `{}`), ErrInvalid, 0},
		{"empty close", legacyResponse(204, "", ""), nil, 204},
		{"nonempty close", legacyResponse(204, "application/json", `{}`), ErrInvalid, 0},
		{"oversize", legacyResponse(200, "application/json", strings.Repeat(" ", 4<<20+1)), ErrInvalid, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(*http.Request) (*http.Response, error) { return tc.response, nil })
			login(t, c)
			s, _ := c.Session()
			out, err := s.LegacyBlender(context.Background(), "GET", "/studio/apps/blender/instances", nil)
			if !errors.Is(err, tc.want) || out.Status != tc.status || len(c.transfers) != 0 {
				t.Fatal("response boundary", err, out.Status)
			}
		})
	}
	for _, mutate := range []func(*http.Response){
		func(r *http.Response) { r.ContentLength++ }, func(r *http.Response) { r.ContentLength-- },
		func(r *http.Response) { r.Header.Set("Content-Encoding", "gzip") },
		func(r *http.Response) {
			r.ContentLength = -1
			r.Body = io.NopCloser(strings.NewReader(strings.Repeat(" ", 4<<20+1)))
		},
	} {
		c := fixture(t, func(*http.Request) (*http.Response, error) {
			r := legacyResponse(200, "application/json", `{}`)
			mutate(r)
			return r, nil
		})
		login(t, c)
		s, _ := c.Session()
		if _, err := s.LegacyBlender(context.Background(), "GET", "/studio/apps/blender/instances", nil); !errors.Is(err, ErrInvalid) {
			t.Fatal("bad response accepted", err)
		}
	}
}

func TestLegacyBlenderCancellationAndLateAuthentication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		calls++
		return legacyResponse(200, "application/json", `{}`), nil
	})
	login(t, c)
	s, _ := c.Session()
	if _, err := s.LegacyBlender(ctx, "GET", "/studio/apps/blender/instances", nil); !errors.Is(err, ErrUnavailable) || calls != 0 {
		t.Fatal("cancelled request sent")
	}
	for _, code := range []int{200, 401} {
		c.transport = memoryTransport(func(*http.Request) (*http.Response, error) {
			c.mu.Lock()
			c.epoch++
			c.authenticated = true
			c.mu.Unlock()
			return legacyResponse(code, "application/json", `{}`), nil
		})
		s, _ = c.Session()
		if _, err := s.LegacyBlender(context.Background(), "GET", "/studio/apps/blender/instances", nil); !errors.Is(err, ErrChanged) || !c.authenticated {
			t.Fatal("late response changed new session")
		}
	}
}

func TestLegacyBlenderAssets(t *testing.T) {
	for _, item := range []struct{ name, kind string }{{"", "text/html"}, {"assets/main.js", "application/javascript"}, {"settings", "application/json"}, {"turn", "application/json"}, {"favicon.ico", "image/x-icon"}} {
		data := "asset"
		if item.kind == "application/json" {
			data = `{"ok":true}`
		}
		c := fixture(t, func(*http.Request) (*http.Response, error) { return legacyResponse(200, item.kind, data), nil })
		login(t, c)
		s, _ := c.Session()
		out, err := s.LegacyBlender(context.Background(), "GET", "/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/"+item.name, nil)
		if err != nil || string(out.Data) != data {
			t.Fatal(item.name, err)
		}
	}
}

func TestLegacyBlenderRejectsBeforeNetworkAndRevokesCurrentLogin(t *testing.T) {
	calls := 0
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		calls++
		return legacyResponse(401, "application/json", `{"code":"UNAUTHENTICATED"}`), nil
	})
	login(t, c)
	s, _ := c.Session()
	if _, err := s.LegacyBlender(context.Background(), "GET", "/studio/apps/blender/instances?override=x", nil); !errors.Is(err, ErrInvalid) || calls != 0 {
		t.Fatal("invalid request reached network")
	}
	if _, err := s.OpenBlenderSignal(context.Background(), "other", strings.Repeat("s", 43)); !errors.Is(err, ErrInvalid) || calls != 0 {
		t.Fatal("invalid socket path reached network")
	}
	if _, err := s.LegacyBlender(context.Background(), "GET", "/studio/apps/blender/instances", nil); !errors.Is(err, ErrDenied) || c.authenticated || len(c.transfers) != 0 {
		t.Fatal("current revoked login retained", err)
	}
	if _, err := s.OpenBlenderSignal(context.Background(), "blenderA", strings.Repeat("s", 43)); !errors.Is(err, ErrChanged) || calls != 1 {
		t.Fatal("revoked signal sent")
	}
}

func TestLegacyBlenderSignalHandshakeFailure(t *testing.T) {
	for _, status := range []int{401, 403, 302, 200} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			c := fixture(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != "/apps/blender/blenderA/"+strings.Repeat("s", 43)+"/webrtc/signalling/" || r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Sec-WebSocket-Protocol") != "" {
					t.Error("incorrect handshake")
				}
				out := legacyResponse(status, "application/json", `{}`)
				if status == 302 {
					out.Header.Set("Location", "https://foreign.invalid/")
				}
				return out, nil
			})
			login(t, c)
			s, _ := c.Session()
			conn, err := s.OpenBlenderSignal(context.Background(), "blenderA", strings.Repeat("s", 43))
			if conn != nil || err == nil || calls != 1 || len(c.transfers) != 0 {
				t.Fatal("failed handshake leaked or retried")
			}
			if c.authenticated != (status != 401) {
				t.Fatal("wrong login revocation")
			}
		})
	}
}
