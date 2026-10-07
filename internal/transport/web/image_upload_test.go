package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestImageUploadMultipartContract(t *testing.T) {
	role := "admin"
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/identity/login" {
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"access_token":"test-session-token"}`)
			return
		}
		_, _ = io.WriteString(w, `{"roles":["`+role+`"]}`)
	}))
	defer core.Close()
	fileData := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0, 10, 13, 255}, 40000)...)
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer test-image-token" {
			t.Error("unexpected credential forwarding")
		}
		if r.URL.Path == "/api/artifacts/upload" {
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Errorf("multipart boundary/body lost: %v", err)
				w.WriteHeader(400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			if r.FormValue("project_id") != "test-existing-project" {
				t.Error("project_id not preserved")
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Errorf("file not forwarded: %v", err)
				w.WriteHeader(400)
				return
			}
			defer file.Close()
			got, _ := io.ReadAll(file)
			if !bytes.Equal(got, fileData) || header.Filename != "outfit-01.png" {
				t.Error("binary contents or filename changed")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer upstream.Close()
	station, _ := application.NewStation(core.URL)
	router, server := NewServer(station, "https://studio.example", fstest.MapFS{})
	server.EnableImage(router, ImageConfig{upstream.URL, "test-image-token"})
	login := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{}`))
	login.Header.Set("Origin", "https://studio.example")
	login.Header.Set("Content-Type", "application/json")
	lw := httptest.NewRecorder()
	router.ServeHTTP(lw, login)
	if len(lw.Result().Cookies()) == 0 {
		t.Fatalf("login failed: %d", lw.Code)
	}
	cookie := lw.Result().Cookies()[0]
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("project_id", "test-existing-project")
	part, _ := form.CreateFormFile("file", "outfit-01.png")
	_, _ = part.Write(fileData)
	_ = form.Close()
	for _, tc := range []struct {
		name, path, contentType, origin, role string
		body                                  []byte
		want                                  int
		forward                               bool
	}{
		{"upload", "/studio/apps/image/api/artifacts/upload", form.FormDataContentType(), "https://studio.example", "admin", body.Bytes(), 200, true},
		{"alias upload", "/apps/image/api/artifacts/upload", form.FormDataContentType(), "https://studio.example", "admin", body.Bytes(), 200, true},
		{"missing boundary", "/apps/image/api/artifacts/upload", "multipart/form-data", "https://studio.example", "admin", body.Bytes(), 415, false},
		{"json is not an upload", "/apps/image/api/artifacts/upload", "application/json", "https://studio.example", "admin", []byte(`{}`), 415, false},
		{"tasks stay json", "/apps/image/api/tasks", "application/json; charset=utf-8", "https://studio.example", "admin", []byte(`{}`), 200, true},
		{"tasks reject multipart", "/apps/image/api/tasks", form.FormDataContentType(), "https://studio.example", "admin", body.Bytes(), 415, false},
		{"origin guard", "/apps/image/api/artifacts/upload", form.FormDataContentType(), "https://untrusted.example", "admin", body.Bytes(), 403, false},
		{"admin guard", "/apps/image/api/artifacts/upload", form.FormDataContentType(), "https://studio.example", "user", body.Bytes(), 403, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			role = tc.role
			before := upstreamCalls
			req := httptest.NewRequest("POST", tc.path, bytes.NewReader(tc.body))
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", tc.contentType)
			req.AddCookie(cookie)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
			if (upstreamCalls > before) != tc.forward {
				t.Error("unexpected upstream forwarding")
			}
		})
	}
}
