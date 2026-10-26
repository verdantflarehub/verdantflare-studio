package web

import (
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"net/http/httptest"
	"os"
	"testing"
)

func TestStableMediaHTTPConditionsAndRanges(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "media")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write(make([]byte, 32))
	f.WriteString("hello")
	media := application.MediaFile{File: f, Size: 5, MIME: "image/webp", ETag: `"hash"`}
	cases := []struct {
		method, rangeHeader, etag, ifRange string
		status                             int
		body                               string
	}{
		{"GET", "", "", "", 200, "hello"},
		{"HEAD", "", "", "", 200, ""},
		{"GET", "bytes=1-3", "", "", 206, "ell"},
		{"GET", "bytes=1-3", "", `"other"`, 200, "hello"},
		{"GET", "", `"hash"`, "", 304, ""},
		{"GET", "bytes=20-30", "", "", 416, ""},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.rangeHeader+tc.etag+tc.ifRange, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "https://studio.test/studio/media/a/b/original-v1", nil)
			r.Header.Set("Range", tc.rangeHeader)
			r.Header.Set("If-None-Match", tc.etag)
			r.Header.Set("If-Range", tc.ifRange)
			w := httptest.NewRecorder()
			serveDashboardMedia(w, r, media)
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			if tc.status < 400 && (w.Body.String() != tc.body || w.Header().Get("Cache-Control") != application.MediaCacheControl || w.Header().Get("ETag") != media.ETag) {
				t.Fatal(w.Header(), w.Body.String())
			}
			if tc.status >= 400 && (w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("ETag") != "") {
				t.Fatal("error cached", w.Header())
			}
			if tc.status == 206 && w.Header().Get("Content-Range") != "bytes 1-3/5" {
				t.Fatal("wrong range", w.Header())
			}
		})
	}
}
