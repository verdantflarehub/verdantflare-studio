package application

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOperationProxyBoundary(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/app-operations/00000000-0000-4000-8000-000000000001" || r.Header.Get("Authorization") != "Bearer test-session" {
			t.Errorf("unexpected upstream request")
		}
		io.WriteString(w, `{"status":"running","phase":"downloading","download":{"downloaded_bytes":25,"total_bytes":100,"percent":25}}`)
	}))
	defer upstream.Close()
	station, err := NewStation(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	path := "operations/00000000-0000-4000-8000-000000000001"
	if out := station.Call(context.Background(), "", Request{Path: path, Method: "GET"}); out.Status != 401 {
		t.Fatal(out.Status)
	}
	for _, bad := range []string{"operations/not-an-id", path + "/extra", path + "?organization_id=other", "operations/../identity/me"} {
		if out := station.Call(context.Background(), "test-session", Request{Path: bad, Method: "GET"}); out.Status != 404 {
			t.Fatalf("allowed path %s", bad)
		}
	}
	if calls != 0 {
		t.Fatal("invalid request reached upstream")
	}
	out := station.Call(context.Background(), "test-session", Request{Path: path, Method: "GET"})
	if out.Status != 200 || calls != 1 {
		t.Fatalf("proxy failed: %d", out.Status)
	}
}
