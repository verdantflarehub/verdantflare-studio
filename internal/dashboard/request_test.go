package dashboard

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDecodeAppRequestPreservesNullableBusinessJSON(t *testing.T) {
	value, err := DecodeRequest(strings.NewReader(`{"method":"POST","path":"/instances/one/start","body":{"profile":null,"items":[]}}`))
	if err != nil || string(value.Body) != `{"profile":null,"items":[]}` {
		t.Fatal("business JSON lost", err)
	}
	for _, input := range []string{
		`{"method":"GET","path":"/instances","body":null}`,
		`{"method":"POST","path":"/instances","body":{"x":1,"x":2}}`,
		`{"Method":"POST","path":"/instances"}`,
		`{"method":"POST","path":"/instances","organization":"other"}`,
		`{"method":null,"path":"/instances"}`, `{"method":"GET","path":null}`,
		`{"method":"GET","path":"/instances?url=external"}`,
		`{"method":"GET","path":"/instances/../private"}`,
		`{"method":"POST","path":"/instances","body":1e1000}`,
		`{"method":"GET","path":"/instances"} {}`,
		"{\"method\":\"POST\",\"path\":\"/instances\",\"body\":\"\xff\"}",
		`{"method":"POST","path":"/instances","body":"` + strings.Repeat("x", 1<<20) + `"}`,
	} {
		if _, err := DecodeRequest(strings.NewReader(input)); err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestAppPolicyControlsProbeDeadline(t *testing.T) {
	f := newRegisteredFixture(t)
	binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
	if err != nil {
		t.Fatal(err)
	}
	binding.Declaration.Dashboard.MinimumSDKVersion = "1.1.0"
	binding.Rules[1].TimeoutSeconds = 60
	f.source.client.Transport = sourceTransport(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) < 59*time.Second || time.Until(deadline) > 60*time.Second {
			t.Fatal("probe deadline truncated")
		}
		if req.Header.Get("X-Station-Id") != f.actor.StationID {
			t.Fatal("station context missing")
		}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	})
	if _, status, err := f.source.Request(context.Background(), f.actor, binding, AppRequest{Method: "POST", Path: "/instances/one/start"}, "probe"); err != nil || status != 204 {
		t.Fatal(status, err)
	}
	if f.source.client.Timeout != 15*time.Second {
		t.Fatal("asset client timeout mutated")
	}
}

func TestAppRequestMapsOnlyMatchedRuleAndPreservesIdentity(t *testing.T) {
	f := newRegisteredFixture(t)
	binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	f.source.client.Transport = sourceTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != "http://sample.apps.svc.cluster.local:8080/internal/instances/one/start" || req.Method != "POST" ||
			req.Header.Get("X-User-Id") != f.actor.UserID || req.Header.Get("X-Organization-Id") != f.actor.OrganizationID ||
			req.Header.Get("X-Request-Id") != "request-one" || req.Header.Get("Authorization") != "Bearer unit-only-service-credential-000000000" {
			t.Fatal("uncontrolled request")
		}
		data, _ := io.ReadAll(req.Body)
		if string(data) != `{"profile":null}` || req.Header.Get("Content-Type") != "application/json" {
			t.Fatal("body changed")
		}
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"operationId":"op-one","result":null}`)), Header: http.Header{"Content-Type": {"application/json"}}}, nil
	})
	data, status, err := f.source.Request(context.Background(), f.actor, binding, AppRequest{Method: "POST", Path: "/instances/one/start", Body: []byte(`{"profile":null}`)}, "request-one")
	if err != nil || status != 202 || string(data) != `{"operationId":"op-one","result":null}` || calls != 1 {
		t.Fatal("operation response lost", err)
	}
	for _, input := range []AppRequest{
		{Method: "POST", Path: "/instances/one/stop"}, {Method: "POST", Path: "/instances/one/start", Body: bytes.Repeat([]byte(" "), 257)},
		{Method: "GET", Path: "/instances/one/start"}, {Method: "POST", Path: "//instances/one/start"},
		{Method: "POST", Path: "/instances/%2e%2e/start"}, {Method: "POST", Path: "/instances/one/start?other=1"},
	} {
		if _, _, err := f.source.Request(context.Background(), f.actor, binding, input, "request-one"); err == nil {
			t.Fatal("unlisted request allowed")
		}
	}
	if calls != 1 {
		t.Fatal("denied request reached upstream")
	}
}

func TestAppResponseKeepsSafeErrorsAndRejectsInternalDetails(t *testing.T) {
	f := newRegisteredFixture(t)
	binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name       string
		status     int
		body, mime string
		wantStatus int
		wantBody   string
	}{
		{"conflict", 409, `{"code":"INSTANCE_BUSY","detail":"internal diagnostic"}`, "application/json", 409, `{"code":"INSTANCE_BUSY"}`},
		{"denied", 403, `{"code":"FORBIDDEN","token":"never relay"}`, "application/json", 403, `{"code":"FORBIDDEN"}`},
		{"no content", 204, "", "", 204, ""},
		{"body on 204", 204, "x", "", 0, ""},
		{"redirect", 302, "", "", 0, ""},
		{"internal failure", 500, `{"code":"INTERNAL","detail":"private"}`, "application/json", 0, ""},
		{"invalid error", 409, `{"code":"internal.url.invalid"}`, "application/json", 0, ""},
		{"missing code", 400, `{"message":"private"}`, "application/json", 0, ""},
		{"wrong mime", 200, `{}`, "text/html", 0, ""},
		{"duplicate JSON", 200, `{"id":"one","id":"two"}`, "application/json", 0, ""},
		{"oversized", 200, strings.Repeat(" ", (4<<20)+1), "application/json", 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			f.source.client.Transport = sourceTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{"Content-Type": {tc.mime}, "Location": {"https://other.invalid"}, "Set-Cookie": {"private=value"}}}, nil
			})
			data, status, err := f.source.Request(context.Background(), f.actor, binding, AppRequest{Method: "GET", Path: "/instances"}, "request-one")
			if tc.wantStatus == 0 {
				if err == nil || len(data) != 0 {
					t.Fatal("unsafe response returned")
				}
			} else if err != nil || status != tc.wantStatus || string(data) != tc.wantBody {
				t.Fatal("response semantics changed", err)
			}
			if calls != 1 {
				t.Fatal("request retried")
			}
		})
	}
}

func TestAppWriteCancellationNeverRetries(t *testing.T) {
	f := newRegisteredFixture(t)
	binding, err := f.source.Lookup(context.Background(), f.actor, "install-a")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	f.source.client.Transport = sourceTransport(func(req *http.Request) (*http.Response, error) { calls++; cancel(); return nil, req.Context().Err() })
	if _, _, err := f.source.Request(ctx, f.actor, binding, AppRequest{Method: "POST", Path: "/instances/one/start"}, "request-one"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("write retried after cancellation")
	}
}
