package studioremote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type memoryTransport func(*http.Request) (*http.Response, error)

func (f memoryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}}
}
func fixture(t *testing.T, extra memoryTransport) *Client {
	t.Helper()
	c, err := New("https://studio.invalid")
	if err != nil {
		t.Fatal(err)
	}
	c.transport = memoryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "studio.invalid" || r.Header.Get("Origin") != "https://studio.invalid" || r.Header.Get("Authorization") != "" {
			t.Fatal("uncontrolled transport")
		}
		if r.URL.Path == "/api/login" {
			resp := jsonResponse(201, `{"user_id":"unit-user"}`)
			resp.Header.Set("Set-Cookie", "vf_studio_session=unit-only-cookie; Path=/; Secure; HttpOnly")
			return resp, nil
		}
		cookie, err := r.Cookie("vf_studio_session")
		if err != nil || cookie.Value != "unit-only-cookie" {
			t.Fatal("server session missing")
		}
		return extra(r)
	})
	t.Cleanup(c.Close)
	return c
}
func login(t *testing.T, c *Client) {
	t.Helper()
	if _, err := c.Send(context.Background(), "POST", "/api/login", []byte(`{"username":"unit","password":"unit-only"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestAppCenterPreservesUpstreamDate(t *testing.T) {
	for _, date := range []string{"Mon, 12 Oct 2026 00:00:00 GMT", "", "invalid"} {
		c := fixture(t, func(r *http.Request) (*http.Response, error) {
			response := jsonResponse(200, `{}`)
			response.Header.Set("Date", date)
			return response, nil
		})
		login(t, c)
		out, err := c.Send(context.Background(), "GET", "/studio/appcenter/v1/view", nil)
		if err != nil || out.Date != date {
			t.Fatal("upstream clock was replaced or lost")
		}
	}
}

func TestRemoteCancelledRequestsDoNotMutateSession(t *testing.T) {
	c := fixture(t, func(r *http.Request) (*http.Response, error) { return jsonResponse(200, `{}`), nil })
	login(t, c)
	epoch := c.epoch
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Send(ctx, "POST", "/api/login", nil); !errors.Is(err, ErrUnavailable) || !c.authenticated || c.epoch != epoch {
		t.Fatal("already cancelled login changed session")
	}
	for _, item := range []struct {
		path          string
		status        int
		authenticated bool
	}{
		{"/api/me", 401, true}, {"/api/login", 201, false}, {"/api/logout", 204, false},
	} {
		t.Run(item.path, func(t *testing.T) {
			c.authenticated = true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.transport = memoryTransport(func(*http.Request) (*http.Response, error) {
				cancel()
				body := `{}`
				if item.status == 204 {
					body = ""
				}
				response := jsonResponse(item.status, body)
				response.Header.Set("Set-Cookie", "vf_studio_session=late-test-session; Path=/; Secure; HttpOnly")
				return response, nil
			})
			method := "POST"
			if item.path == "/api/me" {
				method = "GET"
			}
			if _, err := c.Send(ctx, method, item.path, nil); !errors.Is(err, ErrUnavailable) || c.authenticated != item.authenticated {
				t.Fatal("late cancelled response changed session")
			}
		})
	}
}

func TestRemoteWorkspaceRoutesAndBinaryAssetsAreBounded(t *testing.T) {
	id := "01980000-0000-4000-8000-000000000002"
	assetPath := "/studio/instance-assets/" + strings.Repeat("a", 64) + "/font.woff2"
	for _, route := range []struct{ method, path string }{
		{"POST", "/studio/dashboards/" + id + "/instances/open"}, {"GET", "/studio/instance-workspaces/" + id},
		{"POST", "/studio/instance-workspaces/" + id + "/requests"}, {"DELETE", "/studio/instance-workspaces/" + id}, {"GET", assetPath},
	} {
		if !Allowed(route.method, route.path) {
			t.Fatal("required workspace route missing")
		}
	}
	for _, invalid := range []string{assetPath + "?token=x", strings.Replace(assetPath, "font.woff2", "../secret.json", 1), strings.Replace(assetPath, "font.woff2", "state.sqlite3", 1)} {
		if Allowed("GET", invalid) {
			t.Fatal("unsafe resource accepted")
		}
	}
	wrong := false
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		mime := "font/woff2"
		if wrong {
			mime = "text/html"
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {mime}}, Body: io.NopCloser(strings.NewReader("font-bytes"))}, nil
	})
	login(t, c)
	if out, err := c.Send(context.Background(), "GET", assetPath, nil); err != nil || out.MIME != "font/woff2" {
		t.Fatal(err)
	}
	wrong = true
	if _, err := c.Send(context.Background(), "GET", assetPath, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong asset MIME accepted", err)
	}
}

func TestRemoteCookieIsolationAndLogout(t *testing.T) {
	c := fixture(t, func(r *http.Request) (*http.Response, error) { return jsonResponse(200, `{"items":[]}`), nil })
	if _, err := c.Send(context.Background(), "GET", "/studio/navigation", nil); !errors.Is(err, ErrDenied) {
		t.Fatal("unauthenticated request allowed")
	}
	login(t, c)
	out, err := c.Send(context.Background(), "GET", "/studio/navigation", nil)
	if err != nil || out.Status != 200 || strings.Contains(string(out.Data), "unit-only-cookie") {
		t.Fatal("session not isolated")
	}
	if _, err = c.Send(context.Background(), "POST", "/api/logout", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Send(context.Background(), "GET", "/studio/navigation", nil); !errors.Is(err, ErrDenied) {
		t.Fatal("logout retained session")
	}
}

func TestRemoteConfinesOriginPathsAndRedirects(t *testing.T) {
	for _, origin := range []string{"http://studio.invalid", "https://studio.invalid/path", "https://user@studio.invalid", "https://studio.invalid?", "https://studio.invalid#x"} {
		if _, err := New(origin); err == nil {
			t.Fatal("invalid origin accepted")
		}
	}
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		resp := jsonResponse(302, `{}`)
		resp.Header.Set("Location", "https://elsewhere.invalid/private")
		return resp, nil
	})
	login(t, c)
	for _, path := range []string{"//elsewhere.invalid", "/studio/../api/me", "/studio/dashboards/%2e%2e/assets/index.html", "/api/me?token=x", "/internal/v1/app-access", "/api/identity/bootstrap"} {
		if _, err := c.Send(context.Background(), "GET", path, nil); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid target allowed")
		}
	}
	if calls != 0 {
		t.Fatal("invalid target reached transport")
	}
	if _, err := c.Send(context.Background(), "GET", "/studio/navigation", nil); !errors.Is(err, ErrDenied) || calls != 1 {
		t.Fatal("redirect followed")
	}
}

func TestRemoteLoginInvalidatesInFlightResponses(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		close(started)
		<-release
		return jsonResponse(401, `{"code":"UNAUTHENTICATED"}`), nil
	})
	login(t, c)
	result := make(chan error, 1)
	go func() { _, err := c.Send(context.Background(), "GET", "/studio/navigation", nil); result <- err }()
	<-started
	login(t, c)
	close(release)
	if err := <-result; !errors.Is(err, ErrChanged) {
		t.Fatal("old user response escaped")
	}
	c.mu.Lock()
	authenticated := c.authenticated
	c.mu.Unlock()
	if !authenticated {
		t.Fatal("old 401 cleared new session")
	}
}

func TestRemoteAssetMimeAndResponseLimit(t *testing.T) {
	path := "/studio/dashboards/00000000-0000-4000-8000-000000000001/assets/dashboard/index.html"
	c := fixture(t, func(*http.Request) (*http.Response, error) { return jsonResponse(200, `{}`), nil })
	login(t, c)
	if _, err := c.Send(context.Background(), "GET", path, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("wrong asset MIME accepted")
	}
	c.transport = memoryTransport(func(*http.Request) (*http.Response, error) {
		return jsonResponse(200, strings.Repeat(" ", (4<<20)+1)), nil
	})
	if _, err := c.Send(context.Background(), "GET", "/studio/navigation", nil); !errors.Is(err, ErrUnavailable) {
		t.Fatal("oversized response accepted")
	}
}
