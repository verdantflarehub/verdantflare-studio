package desktop

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

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
