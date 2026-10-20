package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayForwardsOnlySignedMCPToFixedControlPath(t *testing.T) {
	called := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called++
		if req.URL.Path != "/api/control/internal/studio-publish/mcp" || req.Header.Get("X-App-Publish-Signature") != "signature" {
			t.Errorf("unexpected upstream request: %s", req.URL.Path)
		}
		body, _ := io.ReadAll(req.Body)
		if string(body) != `{"method":"tools/call"}` {
			t.Errorf("request body changed: %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer upstream.Close()
	// The production constructor requires HTTPS; this transport maps that fixed
	// host to an in-memory test server without changing the signed request path.
	client := upstream.Client()
	client.Transport = rewriteTransport{base: client.Transport, target: upstream.URL}
	r, err := newRelay("https://hub.verdantflarehub.com/api/control/internal/studio-publish/mcp", client)
	if err != nil {
		t.Fatal(err)
	}

	bad := httptest.NewRecorder()
	r.handler().ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`)))
	if bad.Code != http.StatusUnsupportedMediaType || called != 0 {
		t.Fatalf("unsigned request reached upstream: %d, %d", bad.Code, called)
	}

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"method":"tools/call"}`))
	req.Header.Set("Content-Type", "application/json")
	for _, key := range forwardedHeaders[:5] {
		req.Header.Set(key, "signed")
	}
	req.Header.Set("X-App-Publish-Signature", "signature")
	req.Header.Set("X-Request-Id", "request-1")
	rec := httptest.NewRecorder()
	r.handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted || strings.TrimSpace(rec.Body.String()) != `{"result":"ok"}` || called != 1 {
		t.Fatalf("relay failed: status=%d body=%q calls=%d", rec.Code, rec.Body.String(), called)
	}
}

type rewriteTransport struct {
	base   http.RoundTripper
	target string
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = "http"
	copy.URL.Host = strings.TrimPrefix(r.target, "http://")
	return r.base.RoundTrip(copy)
}
