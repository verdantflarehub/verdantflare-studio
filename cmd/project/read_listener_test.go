package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectPeerListenerExposesOnlyRead(t *testing.T) {
	h := projectReadHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for path, status := range map[string]int{"/internal/v1/project/open": 204, "/mcp": 404, "/internal/v1/project/commit": 404, "/internal/v1/artifact/authorize": 404, "/internal/v1/project/open?x=1": 404, "/internal/v1/project/op%65n": 404, "/internal/v1/project/open/": 404} {
		r := httptest.NewRequest("POST", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
	if validateReadListener("0.0.0.0:8095", "0.0.0.0:8096", true) != nil {
		t.Fatal("valid listener rejected")
	}
	for _, peer := range []string{"0.0.0.0:8095", "bad"} {
		if validateReadListener("0.0.0.0:8095", peer, true) == nil {
			t.Fatal("invalid listener accepted")
		}
	}
	if validateReadListener("0.0.0.0:8095", "0.0.0.0:8096", false) == nil {
		t.Fatal("insecure peer listener accepted")
	}
}
