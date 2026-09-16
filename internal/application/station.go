// Package application is shared by the HTTP and desktop transports.
package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Request struct {
	Path   string          `json:"path"`
	Method string          `json:"method"`
	Body   json.RawMessage `json:"body,omitempty"`
}
type Result struct {
	Status    int             `json:"status"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
	Token     string          `json:"-"`
}
type Station struct {
	base   string
	client *http.Client
}

var appPath = regexp.MustCompile(`^apps/[a-z0-9][a-z0-9-]{0,62}$`)

func ID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func NewStation(base string) (*Station, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, &url.Error{Op: "configure", URL: "Station endpoint", Err: errInvalidURL{}}
	}
	return &Station{strings.TrimRight(base, "/"), &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

type errInvalidURL struct{}

func (errInvalidURL) Error() string { return "invalid Station URL" }
func Error(status int, code string) Result {
	b, _ := json.Marshal(map[string]string{"code": code})
	return Result{Status: status, Data: b, RequestID: ID()}
}
func (s *Station) Call(ctx context.Context, token string, in Request) Result {
	path := ""
	switch {
	case in.Method == "POST" && in.Path == "login":
		path = "/identity/login"
	case in.Method == "POST" && in.Path == "logout":
		path = "/identity/logout"
	case in.Method == "POST" && in.Path == "commands":
		path = "/app-commands"
	case in.Method == "GET" && in.Path == "me":
		path = "/identity/me"
	case in.Method == "GET" && in.Path == "health":
		path = "/station/health"
	case in.Method == "GET" && in.Path == "apps":
		path = "/catalog/apps"
	case in.Method == "GET" && appPath.MatchString(in.Path):
		path = "/catalog/" + in.Path
	default:
		return Error(404, "NOT_FOUND")
	}
	if in.Path != "login" && token == "" {
		return Error(401, "UNAUTHENTICATED")
	}
	if len(in.Body) > 16384 || (len(in.Body) > 0 && !json.Valid(in.Body)) {
		return Error(400, "INVALID_ARGUMENT")
	}
	req, err := http.NewRequestWithContext(ctx, in.Method, s.base+path, bytes.NewReader(in.Body))
	if err != nil {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	id := ID()
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return Result{Status: 204, Data: json.RawMessage(`{}`), RequestID: id}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil || len(b) > 4*1024*1024 {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(b, &data) != nil || data == nil {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	var access string
	if raw, ok := data["access_token"]; ok {
		json.Unmarshal(raw, &access)
		delete(data, "access_token")
	}
	delete(data, "refresh_token")
	if in.Path == "login" && resp.StatusCode == 201 && access == "" {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	// Do not relay untrusted internal error details to either UI.
	if resp.StatusCode >= 500 {
		return Error(503, "SERVICE_UNAVAILABLE")
	}
	b, _ = json.Marshal(data)
	return Result{Status: resp.StatusCode, Data: b, RequestID: id, Token: access}
}
