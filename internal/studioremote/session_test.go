package studioremote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

const artifactPath = "/v2/artifacts/00000000-0000-7000-8000-000000000003/content"
const artifactQuery = "store_id=00000000-0000-7000-8000-000000000001&artifact_id=00000000-0000-7000-8000-000000000002"

func TestRemoteWorkspaceSessionCannotFollowNewLogin(t *testing.T) {
	calls := 0
	c := fixture(t, func(*http.Request) (*http.Response, error) { calls++; return jsonResponse(200, `{}`), nil })
	login(t, c)
	s, err := c.Session()
	if err != nil {
		t.Fatal(err)
	}
	login(t, c)
	if _, err := s.Send(context.Background(), "POST", "/mcp", []byte(`{}`)); !errors.Is(err, ErrChanged) {
		t.Fatal("old workspace acquired new login")
	}
	if _, err := s.Transfer(context.Background(), "GET", artifactPath, artifactQuery, nil, 0, 128); !errors.Is(err, ErrChanged) {
		t.Fatal("old transfer acquired new login")
	}
	if calls != 0 {
		t.Fatal("stale workspace reached server")
	}
}

func TestRemoteWorkspaceTransferIsBoundedAndScoped(t *testing.T) {
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatal("compressed transfer enabled")
		}
		return &http.Response{StatusCode: 200, ContentLength: 5, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader("hello"))}, nil
	})
	login(t, c)
	s, _ := c.Session()
	for _, query := range []string{artifactQuery + "&token=secret", artifactQuery + "&project_id=00000000-0000-7000-8000-000000000004", artifactQuery + "&store_id=00000000-0000-7000-8000-000000000001"} {
		if _, err := s.Transfer(context.Background(), "GET", artifactPath, query, nil, 0, 8); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid scope reached transport")
		}
	}
	if calls != 0 {
		t.Fatal("invalid transfer sent")
	}
	if _, err := s.Transfer(context.Background(), "GET", artifactPath, artifactQuery, nil, 0, 4); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized download accepted")
	}
	resp, err := s.Transfer(context.Background(), "GET", artifactPath, artifactQuery, nil, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil || string(data) != "hello" {
		t.Fatal("download failed")
	}
}

func TestRemoteWorkspaceLogoutCancelsBinaryAndRejectsBytes(t *testing.T) {
	var transferContext context.Context
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		transferContext = r.Context()
		return &http.Response{StatusCode: 200, ContentLength: 5, Header: http.Header{"Content-Type": {"application/octet-stream"}}, Body: io.NopCloser(strings.NewReader("hello"))}, nil
	})
	login(t, c)
	s, _ := c.Session()
	resp, err := s.Transfer(context.Background(), "GET", artifactPath, artifactQuery, nil, 0, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	login(t, c)
	if transferContext.Err() == nil {
		t.Fatal("old stream not canceled")
	}
	if _, err := io.ReadAll(resp.Body); !errors.Is(err, ErrChanged) {
		t.Fatal("old stream bytes escaped")
	}
}

func TestRemoteWorkspaceUploadDoesNotOwnSource(t *testing.T) {
	source := &ownedSource{Reader: strings.NewReader("hello")}
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.ContentLength != 5 {
			t.Fatal("upload framing changed")
		}
		data, err := io.ReadAll(r.Body)
		if err != nil || string(data) != "hello" {
			t.Fatal("upload failed")
		}
		_ = r.Body.Close()
		return jsonResponse(200, `{}`), nil
	})
	login(t, c)
	s, _ := c.Session()
	resp, err := s.Transfer(context.Background(), "PUT", "/v2/artifacts/uploads/00000000-0000-7000-8000-000000000003/content", "", source, 5, 5)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if source.closed {
		t.Fatal("transport closed workspace source")
	}
}

type ownedSource struct {
	*strings.Reader
	closed bool
}

func (s *ownedSource) Close() error { s.closed = true; return nil }

func TestRemoteStableMediaRangeAndConditionalHeaders(t *testing.T) {
	path := "/studio/media/" + strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64) + "/original-v1"
	calls := 0
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Range") != "bytes=1-3" || r.Header.Get("If-Range") != `"hash"` || r.Header.Get("X-Unsafe") != "" {
			t.Fatal("media request headers", r.Header)
		}
		return &http.Response{StatusCode: 206, ContentLength: 3, Header: http.Header{"Content-Type": {"video/mp4"}, "Content-Range": {"bytes 1-3/5"}, "Cache-Control": {"private, max-age=2592000, immutable"}, "Etag": {`"hash"`}}, Body: io.NopCloser(strings.NewReader("ell"))}, nil
	})
	login(t, c)
	s, _ := c.Session()
	headers := http.Header{"Range": {"bytes=1-3"}, "If-Range": {`"hash"`}, "X-Unsafe": {"skip"}}
	response, err := s.Media(context.Background(), "GET", path, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ell" || response.StatusCode != 206 || response.Header.Get("Content-Range") != "bytes 1-3/5" || response.Header.Get("Cache-Control") == "" {
		t.Fatal("range response", err, response)
	}
	headers["Range"] = []string{"bytes=0-1", "bytes=2-3"}
	if _, err = s.Media(context.Background(), "GET", path, headers); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate range accepted")
	}
	login(t, c)
	if _, err = s.Media(context.Background(), "GET", path, nil); !errors.Is(err, ErrChanged) {
		t.Fatal("stale media used new identity")
	}
	if calls != 1 {
		t.Fatal("unexpected network", calls)
	}
}
func TestRemoteStableMedia304IsNotRedirect(t *testing.T) {
	path := "/studio/media/" + strings.Repeat("a", 64) + "/" + strings.Repeat("b", 64) + "/thumbnail-320-v1"
	c := fixture(t, func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("If-None-Match") != `"hash"` {
			t.Fatal("condition lost")
		}
		return &http.Response{StatusCode: 304, Header: http.Header{"Etag": {`"hash"`}, "Cache-Control": {"private, max-age=2592000, immutable"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	login(t, c)
	s, _ := c.Session()
	r, err := s.Media(context.Background(), "GET", path, http.Header{"If-None-Match": {`"hash"`}})
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 304 || r.Header.Get("ETag") != `"hash"` {
		t.Fatal("conditional response lost")
	}
}
