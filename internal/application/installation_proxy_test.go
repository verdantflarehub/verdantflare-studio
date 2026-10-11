package application

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInstallationReadProxyAllowlist(t *testing.T) {
	id := "00000000-0000-7000-8000-000000000001"
	calls := 0
	s := &Station{base: "https://station.invalid", client: &http.Client{Transport: dashboardIdentityTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "GET" || (r.URL.Path != "/app-installations" && r.URL.Path != "/app-installations/"+id) || r.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("unsafe installation proxy request")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"items":[]}`))}, nil
	})}}
	for _, path := range []string{"installations", "installations/" + id} {
		out := s.Call(context.Background(), "test-session", Request{Path: path, Method: "GET"})
		if out.Status != 200 {
			t.Fatal(out.Status)
		}
	}
	for _, request := range []Request{
		{Path: "installations", Method: "POST"}, {Path: "installations/" + id, Method: "DELETE"},
		{Path: "installations?organization_id=other", Method: "GET"}, {Path: "installations/../me", Method: "GET"},
		{Path: "installations/" + id + "/dashboard", Method: "GET"}, {Path: "installations/" + id + "?url=other", Method: "GET"},
	} {
		if out := s.Call(context.Background(), "test-session", request); out.Status != 404 {
			t.Fatal("unknown installation request accepted", out.Status)
		}
	}
	if calls != 2 {
		t.Fatal("invalid input reached Core")
	}
}
