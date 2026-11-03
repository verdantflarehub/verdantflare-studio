package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"testing"
)

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type diagnosticBody struct {
	io.Reader
	err    error
	closed bool
}

func (b *diagnosticBody) Read(p []byte) (int, error) {
	if b.err != nil {
		return 0, b.err
	}
	return b.Reader.Read(p)
}
func (b *diagnosticBody) Close() error { b.closed = true; return nil }

func TestCallFailureDiagnosticsPreservePublicErrorsAndDoNotReplay(t *testing.T) {
	const requestID = "00000000-0000-7000-8000-000000000001"
	const private = "private-sentinel"
	valid := `{"jsonrpc":"2.0","id":"` + requestID + `","result":{"content":[]}}`
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", private)
	cases := []struct {
		name, body, stage, reason, public string
		transportErr, readErr             error
		status                            int
	}{
		{name: "connection refused", transportErr: syscall.ECONNREFUSED, stage: "transport", reason: "connection_refused", public: "downstream unavailable"},
		{name: "connection reset", transportErr: syscall.ECONNRESET, stage: "transport", reason: "connection_reset", public: "downstream unavailable"},
		{name: "timeout", transportErr: context.DeadlineExceeded, stage: "transport", reason: "timeout", public: "downstream unavailable"},
		{name: "cancelled", transportErr: context.Canceled, stage: "transport", reason: "cancelled", public: "downstream unavailable"},
		{name: "dns", transportErr: &net.DNSError{Name: private, Err: private}, stage: "transport", reason: "dns", public: "downstream unavailable"},
		{name: "unknown", transportErr: errors.New(private), stage: "transport", reason: "transport_unknown", public: "downstream unavailable"},
		{name: "read truncated", status: 200, readErr: io.ErrUnexpectedEOF, stage: "read", reason: "truncated", public: "invalid downstream response"},
		{name: "read timeout", status: 200, readErr: context.DeadlineExceeded, stage: "read", reason: "timeout", public: "invalid downstream response"},
		{name: "too large", status: 200, body: strings.Repeat("x", (12<<20)+1), stage: "read", reason: "response_too_large", public: "invalid downstream response"},
		{name: "redirect", status: 307, body: private, stage: "protocol", reason: "redirect", public: "downstream redirect rejected"},
		{name: "invalid json", status: 502, body: private, stage: "protocol", reason: "invalid_json", public: "invalid downstream response"},
		{name: "rpc error", status: 200, body: `{"error":{"message":"` + private + `"}}`, stage: "protocol", reason: "rpc_error", public: "downstream protocol error"},
		{name: "wrong identity", status: 200, body: strings.Replace(valid, requestID, private, 1), stage: "protocol", reason: "identity_mismatch", public: "downstream response identity mismatch"},
		{name: "missing result", status: 200, body: `{"private":"` + private + `"}`, stage: "protocol", reason: "invalid_envelope", public: "invalid downstream response"},
		{name: "success", status: 200, body: valid},
		{name: "business failure passthrough", status: 403, body: valid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			g := NewGatewayWithClient(nil)
			g.diagnosticLogger = slog.New(slog.NewJSONHandler(&log, nil))
			g.services["project"] = ServiceRegistration{Endpoint: "https://" + private + "/mcp", Tools: []ToolDefinition{{Name: "project.open"}}}
			calls := 0
			body := &diagnosticBody{Reader: strings.NewReader(tc.body), err: tc.readErr}
			g.httpClient.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if tc.transportErr != nil {
					return nil, &url.Error{Op: "Post", URL: "https://" + private, Err: tc.transportErr}
				}
				return &http.Response{StatusCode: tc.status, Body: body, Header: http.Header{}, Request: r}, nil
			})
			result, status, err := g.call(context.Background(), "project.open", map[string]any{"private": private}, map[string]string{"X-Request-Id": requestID})
			if calls != 1 {
				t.Fatalf("request replayed: %d calls", calls)
			}
			if tc.transportErr == nil && !body.closed {
				t.Fatal("response not closed")
			}
			if tc.public == "" {
				if err != nil || result == nil || status != tc.status || log.Len() != 0 {
					t.Fatalf("changed normal response: status=%d err=%v log=%s", status, err, &log)
				}
				return
			}
			if status != 502 || result != nil || err == nil || err.Error() != tc.public {
				t.Fatalf("changed public failure: status=%d err=%v", status, err)
			}
			if strings.Contains(log.String(), private) || strings.Contains(log.String(), "https://") || bytes.Count(log.Bytes(), []byte("\n")) != 1 {
				t.Fatal("private data or duplicate log")
			}
			var entry map[string]any
			if err := json.Unmarshal(log.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry["request_id"] != requestID || entry["tool"] != "project.open" || entry["stage"] != tc.stage || entry["reason"] != tc.reason || entry["downstream_status"] != float64(tc.status) || entry["elapsed_ms"].(float64) < 0 {
				t.Fatalf("incorrect failure evidence: %v", entry)
			}
		})
	}
}

func TestCallFailureDiagnosticsRejectArbitraryRequestID(t *testing.T) {
	var log bytes.Buffer
	g := NewGatewayWithClient(nil)
	g.diagnosticLogger = slog.New(slog.NewJSONHandler(&log, nil))
	g.services["image"] = ServiceRegistration{Endpoint: "https://example.invalid/mcp", Tools: []ToolDefinition{{Name: "image.create"}}}
	calls := 0
	g.httpClient.Transport = diagnosticTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("private upstream body")
	})
	_, _, _ = g.call(context.Background(), "image.create", nil, map[string]string{"X-Request-Id": "private-sentinel\nforged-log"})
	if calls != 1 || strings.Contains(log.String(), "private") || strings.Contains(log.String(), "forged") || !strings.Contains(log.String(), `"request_id":"unavailable"`) {
		t.Fatalf("unsafe diagnostics or replay: %s", &log)
	}
}
