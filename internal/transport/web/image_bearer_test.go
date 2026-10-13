package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestImageContentBearer(t *testing.T) {
	const token = "existing-mcp-user-token"
	const path = "/apps/image/artifacts/art-0123456789abcdef/content"
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/artifacts/art-0123456789abcdef/content" || r.Header.Get("Authorization") != "Bearer image-service-token" || r.Header.Get("Cookie") != "" || r.Header.Get("X-User-Id") != "" {
			t.Error("unexpected upstream path or forwarded external identity")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("image-content"))
	}))
	defer upstream.Close()
	router, server := NewServer(bearerStation(t, token), "https://studio.example", fstest.MapFS{})
	server.EnableImage(router, ImageConfig{upstream.URL, "image-service-token"})
	for _, tc := range []struct {
		name, method, path, auth, body, site, user string
		want                                       int
	}{
		{"valid", "GET", path, "Bearer " + token, "", "", "", 200},
		{"alias", "GET", "/studio" + path, "Bearer " + token, "", "", "", 200},
		{"anonymous", "GET", path, "", "", "", "", 401},
		{"invalid", "GET", path, "Bearer wrong", "", "", "", 401},
		{"cross-site", "GET", path, "Bearer " + token, "", "cross-site", "", 403},
		{"same-site", "GET", path, "Bearer " + token, "", "same-site", "", 403},
		{"query", "GET", path + "?url=anything", "Bearer " + token, "", "", "", 400},
		{"body", "GET", path, "Bearer " + token, "{}", "", "", 400},
		{"encoded", "GET", strings.Replace(path, "art-", "%61rt-", 1), "Bearer " + token, "", "", "", 404},
		{"forged-identity", "GET", path, "Bearer " + token, "", "", "forged", 403},
		{"dashboard-not-expanded", "GET", "/apps/image/api/tasks", "Bearer " + token, "", "", "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.auth != "" {
				r.Header.Set("Authorization", tc.auth)
			}
			r.Header.Set("Sec-Fetch-Site", tc.site)
			r.Header.Set("X-User-Id", tc.user)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status = %d; want %d", w.Code, tc.want)
			}
			if tc.want == 200 {
				if w.Body.String() != "image-content" || calls != before+1 {
					t.Fatal("content not returned")
				}
			} else if calls != before {
				t.Fatal("denied request reached Image")
			}
		})
	}
}
