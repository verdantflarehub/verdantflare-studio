package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type dashboardUploaderStub struct {
	*dashboardWriterStub
	bytes string
}

func (s *dashboardUploaderStub) PrepareContentUpload(_ context.Context, token, id string, in application.DashboardUploadRequest, _ application.DashboardContentGateway) application.Result {
	s.record(token, "prepare:"+id)
	return application.Result{Status: 200, RequestID: "upload-test", Data: json.RawMessage(`{"upload":{}}`)}
}
func (s *dashboardUploaderStub) UploadContent(_ context.Context, token, id string, in application.DashboardUploadRequest, body io.Reader, length int64, _ application.DashboardContentGateway) application.Result {
	s.record(token, "upload:"+id)
	data, err := io.ReadAll(body)
	if err != nil || int64(len(data)) != length || length != in.Size {
		s.t.Fatal("wrong request body")
	}
	s.bytes = string(data)
	return application.Result{Status: 200, RequestID: "upload-test", Data: json.RawMessage(`{"version":{}}`)}
}

func TestDashboardUploadRouteKeepsCookieOriginAndMetadataBoundaries(t *testing.T) {
	r, server := NewServer(nil, "https://studio.invalid", fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("shell")}})
	server.sessions["browser-session"] = session{token: "server-owned-session", expires: time.Now().Add(time.Hour)}
	stub := &dashboardUploaderStub{dashboardWriterStub: &dashboardWriterStub{dashboardWebStub: &dashboardWebStub{t: t}}}
	server.EnableDashboards(r, stub)
	id := uuid.Must(uuid.NewV7()).String()
	sid := uuid.NewString()
	in := application.DashboardUploadRequest{Mode: "prepare", WriteID: id, Source: project.Source{Kind: "user_import", WorldDraftID: id}, MIME: "application/json", Size: 2, SHA256: strings.Repeat("a", 64)}
	metadata, _ := json.Marshal(in)
	prepare := "/studio/dashboards/" + sid + "/capabilities/artifact.write"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, dashboardWebRequest("POST", prepare, string(metadata)))
	if w.Code != 200 || len(stub.calls) != 1 {
		t.Fatal("prepare failed", w.Code)
	}
	path := "/studio/dashboards/" + sid + "/uploads/" + id
	request := func() *http.Request {
		req := dashboardWebRequest("PUT", path, "{}")
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-Artifact-Metadata", string(metadata))
		return req
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request())
	if w.Code != 200 || len(stub.calls) != 2 || stub.bytes != "{}" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("binary route failed", w.Code)
	}
	for name, mutate := range map[string]func(*http.Request){
		"no cookie":      func(r *http.Request) { r.Header.Del("Cookie") },
		"foreign origin": func(r *http.Request) { r.Header.Set("Origin", "https://other.invalid") },
		"bearer":         func(r *http.Request) { r.Header.Set("Authorization", "Bearer forbidden") },
		"wrong type":     func(r *http.Request) { r.Header.Set("Content-Type", "application/json") },
		"unknown length": func(r *http.Request) { r.ContentLength = -1 },
		"over limit":     func(r *http.Request) { r.ContentLength = application.DashboardUploadLimit + 1 },
		"wrong write": func(r *http.Request) {
			r.URL.Path = strings.Replace(r.URL.Path, id, uuid.Must(uuid.NewV7()).String(), 1)
		},
		"duplicate metadata": func(r *http.Request) { r.Header.Add("X-Artifact-Metadata", string(metadata)) },
		"missing metadata":   func(r *http.Request) { r.Header.Del("X-Artifact-Metadata") },
		"size mismatch":      func(r *http.Request) { r.ContentLength = 1 },
		"encoded body":       func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") },
		"query":              func(r *http.Request) { r.URL.RawQuery = "scope=other" },
	} {
		t.Run(name, func(t *testing.T) {
			req := request()
			mutate(req)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code < 400 || len(stub.calls) != 2 {
				t.Fatal("invalid upload reached service", w.Code)
			}
		})
	}
}
