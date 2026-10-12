package desktop

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

// Runs on 5090: this exercises the real HTTP relay, including automatic Date headers.
func TestRemoteAppCenterDateRelay(t *testing.T) {
	for _, date := range []string{"Mon, 12 Oct 2026 00:00:00 GMT", "", "invalid"} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header()["Date"] = nil
			if date != "" {
				w.Header().Set("Date", date)
			}
			if r.URL.Path == "/api/login" {
				w.Header().Set("Set-Cookie", "vf_studio_session=unit-cookie; Path=/; HttpOnly")
				w.WriteHeader(201)
			} else {
				w.Header().Set("X-Request-ID", "clock-test")
			}
			_, _ = w.Write([]byte(`{}`))
		}))
		remote, err := studioremote.New(upstream.URL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = remote.Send(context.Background(), "POST", "/api/login", []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
		relay := httptest.NewServer(RemoteAssetHandler(NewRemote(remote), http.NotFoundHandler()))
		response, err := http.Get(relay.URL + "/studio/appcenter/v1/view")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		relay.Close()
		remote.Close()
		upstream.Close()
		if response.StatusCode != 200 || response.Header.Get("Date") != date || response.Header.Get("X-Request-ID") != "clock-test" {
			t.Fatal("relay replaced upstream timing or request binding")
		}
	}
}

func TestRemoteAssetHandlerIsFailClosedAndPreservesShell(t *testing.T) {
	remote, err := studioremote.New("https://studio.invalid")
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	service := NewRemote(remote)
	fallbacks := 0
	handler := RemoteAssetHandler(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fallbacks++; w.WriteHeader(200) }))
	for _, tc := range []struct {
		path, origin, auth string
		status             int
	}{
		{"/", "", "", 200},
		{"/studio/navigation", "", "", 401},
		{"/studio/navigation", "https://foreign.invalid", "", 401},
		{"/studio/navigation", "", "Bearer browser-value", 400},
		{"/studio/navigation?upstream=outside", "", "", 400},
	} {
		r := httptest.NewRequest("GET", "http://wails.localhost"+tc.path, nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if tc.auth != "" {
			r.Header.Set("Authorization", tc.auth)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status || w.Header().Get("Set-Cookie") != "" {
			t.Fatalf("status %d expected %d", w.Code, tc.status)
		}
	}
	if fallbacks != 1 {
		t.Fatal("remote failure fell back to shell HTML")
	}
}
