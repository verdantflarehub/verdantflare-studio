package studioremote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func legacyResponse(status int, kind, body string) *http.Response {
	return &http.Response{StatusCode: status, ContentLength: int64(len(body)), Header: http.Header{"Content-Type": {kind}}, Body: io.NopCloser(strings.NewReader(body))}
}

func TestLegacyMediaRoutePolicy(t *testing.T) {
	for _, tc := range []struct{ adapter, method, target, kind string }{
		{"image-iframe-v1", "GET", "/dashboard?embed=1&theme=light&view=2", "text/html"},
		{"image-iframe-v1", "GET", "/dashboard/static/dashboard.js", "text/javascript"},
		{"image-iframe-v1", "GET", "/api/tasks?limit=50&offset=0&project_id=project", "application/json"},
		{"image-iframe-v1", "GET", "/api/tasks/stats", "application/json"},
		{"image-iframe-v1", "POST", "/api/artifacts/upload", "application/json"},
		{"video-iframe-v1", "GET", "/dashboard/tasks/task_1", "text/html"},
		{"video-iframe-v1", "GET", "/dashboard/frontend/studio-embed.js", "text/javascript"},
		{"video-iframe-v1", "GET", "/dashboard/frontend/model-catalog.js", "text/javascript"},
		{"video-iframe-v1", "GET", "/dashboard/frontend/model-catalog.css", "text/css"},
		{"video-iframe-v1", "GET", "/api/model-catalog", "application/json"},
		{"video-iframe-v1", "GET", "/api/dashboard?q=%E6%B5%8B%E8%AF%95&page=1&page_size=24&service=&status=&project_id=", "application/json"},
		{"video-iframe-v1", "GET", "/api/models/model-1/instances/worker/gpus/GPU-1:0?window=1h", "application/json"},
		{"video-iframe-v1", "POST", "/api/tasks/task_1/result", "application/json"},
		{"video-iframe-v1", "POST", "/api/tasks/task_1/title", "application/json"},
		{"video-iframe-v1", "GET", "/api/tasks/task_1/thumbnail", "binary"},
		{"video-iframe-v1", "GET", "/api/tasks/task_1/content", "binary"},
		{"video-iframe-v1", "GET", "/artifacts/artifact_1/content", "binary"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			out, err := legacyRoute(tc.adapter, tc.method, tc.target)
			if err != nil || out.kind != tc.kind || !strings.HasPrefix(out.path, "/studio/apps/") {
				t.Fatalf("route: %#v %v", out, err)
			}
		})
	}
	for _, target := range []string{
		"/api/model-catalog/extra", "/api/model-catalog?upstream=foreign", "/dashboard/frontend/model-catalog-secret.js",
		"https://foreign.invalid/api/dashboard", "//foreign.invalid/api/dashboard", "/api/%64ashboard", "/api/dashboard/..", "/api//dashboard", "/api/dashboard?", "/api/dashboard#tasks",
		"/api/dashboard?token=secret", "/api/dashboard?q=1&q=2", "/api/dashboard?q=%00", "/api/dashboard?q=%5c", "/api/dashboard?q=%", "/api/dashboard?q=one;two", "/api/dashboard?q=" + strings.Repeat("a", 513),
		"/dashboard?upstream=https://foreign.invalid", "/api/models/arbitrary/path", "/mcp", "/dashboard/frontend/arbitrary.js", "/artifacts/a/content?token=x",
	} {
		if _, err := legacyRoute("video-iframe-v1", "GET", target); err == nil {
			t.Fatalf("accepted %q", target)
		}
	}
	for _, tc := range []struct{ adapter, method, target string }{
		{"blender-host-v1", "GET", "/dashboard"}, {"video-iframe-v1", "DELETE", "/api/tasks"}, {"image-iframe-v1", "GET", "/api/capabilities"},
		{"video-iframe-v1", "POST", "/api/model-catalog"},
		{"image-iframe-v1", "GET", "/api/tasks/task/thumbnail"},
		{"image-iframe-v1", "GET", "/api/tasks/task/content"},
		{"video-iframe-v1", "POST", "/api/tasks/task/content"}, {"video-iframe-v1", "POST", "/api/models"},
	} {
		if _, err := legacyRoute(tc.adapter, tc.method, tc.target); err == nil {
			t.Fatal("unregistered route accepted")
		}
	}
}

func TestLegacyMediaPreservesFixedRequestAndStripsRemoteHeaders(t *testing.T) {
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/studio/apps/video/api/tasks" || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatal("request changed")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"request_id":"original","prompt":"hello"}` {
			t.Fatal("original command changed")
		}
		out := legacyResponse(201, "application/json", `{"task_id":"original"}`)
		out.Header.Set("Set-Cookie", "remote-cookie=not-for-browser; Secure")
		out.Header.Set("Location", "https://foreign.invalid")
		out.Header.Set("Content-Security-Policy", "unsafe-policy")
		return out, nil
	})
	login(t, c)
	s, _ := c.Session()
	out, err := s.LegacyMedia(context.Background(), "video-iframe-v1", "POST", "/api/tasks", []byte(`{"request_id":"original","prompt":"hello"}`), "application/json", "")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil || string(data) != `{"task_id":"original"}` || out.StatusCode != 201 || out.Request != nil {
		t.Fatal("response changed")
	}
	for _, name := range []string{"Set-Cookie", "Location", "Content-Security-Policy"} {
		if out.Header.Get(name) != "" {
			t.Fatal("remote response header exposed")
		}
	}
	if len(c.transfers) != 0 {
		t.Fatal("buffered transfer leaked")
	}
}

func TestLegacyMediaRejectsBadRequestsBeforeSending(t *testing.T) {
	calls := 0
	c := fixture(t, func(*http.Request) (*http.Response, error) {
		calls++
		return legacyResponse(200, "application/json", `{}`), nil
	})
	login(t, c)
	s, _ := c.Session()
	for _, tc := range []struct{ adapter, method, path, body, kind, byteRange string }{
		{"video-iframe-v1", "GET", "/api/dashboard", `{}`, "", ""},
		{"video-iframe-v1", "GET", "/api/dashboard", "", "", "bytes=0-1"},
		{"video-iframe-v1", "POST", "/api/tasks", `{}`, "text/plain", ""},
		{"video-iframe-v1", "POST", "/api/tasks", `{"a":1,"a":2}`, "application/json", ""},
		{"video-iframe-v1", "POST", "/api/tasks", `{}`, "application/json; upstream=other", ""},
		{"video-iframe-v1", "POST", "/api/tasks", strings.Repeat("a", 1<<20+1), "application/json", ""},
		{"image-iframe-v1", "POST", "/api/artifacts/upload", `{}`, "application/json", ""},
		{"image-iframe-v1", "POST", "/api/artifacts/upload", `body`, "multipart/form-data", ""},
		{"video-iframe-v1", "GET", "/artifacts/a/content", "", "", "bytes=0-1,3-4"},
		{"video-iframe-v1", "GET", "/artifacts/a/content", "", "", "bytes=3-1"},
		{"video-iframe-v1", "GET", "/artifacts/a/content", "", "", "bytes=-0"},
	} {
		if out, err := s.LegacyMedia(context.Background(), tc.adapter, tc.method, tc.path, []byte(tc.body), tc.kind, tc.byteRange); out != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal("bad request accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.LegacyMedia(ctx, "video-iframe-v1", "GET", "/api/dashboard", nil, "", ""); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("invalid or cancelled request sent")
	}
}

func TestLegacyMediaStreamingRangeAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, requestRange, contentRange, body string
		length                                 int64
		ok                                     bool
	}{
		{"bounded", "bytes=2-4", "bytes 2-4/10", "234", 3, true},
		{"open", "bytes=7-", "bytes 7-9/10", "789", 3, true},
		{"suffix", "bytes=-3", "bytes 7-9/10", "789", 3, true},
		{"clamped", "bytes=7-100", "bytes 7-9/10", "789", 3, true},
		{"wrong offset", "bytes=2-4", "bytes 3-5/10", "345", 3, false},
		{"wrong length", "bytes=2-4", "bytes 2-4/10", "234", 2, false},
		{"unsolicited partial", "", "bytes 2-4/10", "234", 3, false},
		{"overflow", "bytes=2-4", "bytes 2-4/99999999999999999999999", "234", 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture(t, func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("Range") != tc.requestRange {
					t.Fatal("range changed")
				}
				out := legacyResponse(206, "video/mp4", tc.body)
				out.ContentLength = tc.length
				out.Header.Set("Content-Range", tc.contentRange)
				return out, nil
			})
			login(t, c)
			s, _ := c.Session()
			out, err := s.LegacyMedia(context.Background(), "video-iframe-v1", "GET", "/artifacts/a/content", nil, "", tc.requestRange)
			if !tc.ok {
				if err == nil {
					out.Body.Close()
					t.Fatal("invalid partial accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer out.Body.Close()
			data, err := io.ReadAll(out.Body)
			if err != nil || string(data) != tc.body {
				t.Fatal("range payload mismatch")
			}
		})
	}
	// Exercise the actual bounded reader without allocating a GiB fixture.
	c := fixture(t, func(*http.Request) (*http.Response, error) { return nil, ErrUnavailable })
	login(t, c)
	s, _ := c.Session()
	for _, tc := range []struct {
		length, limit int64
		body          string
		want          error
	}{
		{3, 4, "ab", io.ErrUnexpectedEOF}, {2, 4, "abc", ErrInvalid}, {-1, 2, "abc", ErrInvalid}, {-1, 3, "abc", nil},
	} {
		b := &legacyBoundBody{ctx: context.Background(), body: io.NopCloser(strings.NewReader(tc.body)), session: s, remaining: tc.limit, expected: tc.length, finish: func() {}}
		_, err := io.ReadAll(b)
		if !errors.Is(err, tc.want) {
			t.Fatalf("length enforcement: %v want %v", err, tc.want)
		}
		b.Close()
	}
}

func TestLegacyMediaSessionRevocationAndCancellation(t *testing.T) {
	var upstream context.Context
	status := 200
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		upstream = r.Context()
		return legacyResponse(status, "image/png", "image-bytes"), nil
	})
	login(t, c)
	old, _ := c.Session()
	out, err := old.LegacyMedia(context.Background(), "image-iframe-v1", "GET", "/artifacts/a/content", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	login(t, c)
	if upstream.Err() == nil {
		t.Fatal("login did not cancel old transfer")
	}
	if _, err = io.ReadAll(out.Body); !errors.Is(err, ErrChanged) {
		t.Fatal("old body survived login")
	}
	out.Body.Close()
	if _, err = old.LegacyMedia(context.Background(), "image-iframe-v1", "GET", "/dashboard", nil, "", ""); !errors.Is(err, ErrChanged) {
		t.Fatal("old session acquired new login")
	}
	s, _ := c.Session()
	ctx, cancel := context.WithCancel(context.Background())
	out, err = s.LegacyMedia(ctx, "image-iframe-v1", "GET", "/artifacts/a/content", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if _, err = io.ReadAll(out.Body); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancelled body readable")
	}
	out.Body.Close()
	if len(c.transfers) != 0 {
		t.Fatal("transfer leaked")
	}
	status = 401
	if _, err = s.LegacyMedia(context.Background(), "image-iframe-v1", "GET", "/artifacts/a/content", nil, "", ""); !errors.Is(err, ErrDenied) || c.authenticated {
		t.Fatal("401 did not revoke login")
	}
}

func TestLegacyMediaRejectsRedirectWrongMIMEAndOversizedResponse(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		kind, body string
		length     int64
	}{
		{"redirect", 302, "text/html", "", 0}, {"html as media", 200, "text/html", "<html>", 6},
		{"oversized", 200, "image/png", "", legacyMediaLimit + 1}, {"bad JSON", 200, "application/json", "not-json", 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c := fixture(t, func(*http.Request) (*http.Response, error) {
				calls++
				out := legacyResponse(tc.status, tc.kind, tc.body)
				out.ContentLength = tc.length
				out.Header.Set("Location", "https://foreign.invalid")
				return out, nil
			})
			login(t, c)
			s, _ := c.Session()
			path := "/artifacts/a/content"
			if tc.name == "bad JSON" {
				path = "/api/dashboard"
			}
			if out, err := s.LegacyMedia(context.Background(), "video-iframe-v1", "GET", path, nil, "", ""); err == nil {
				out.Body.Close()
				t.Fatal("invalid response accepted")
			}
			if calls != 1 || len(c.transfers) != 0 {
				t.Fatal("redirect followed or transfer leaked")
			}
		})
	}
}
