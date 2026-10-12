package studioremote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var legacyBlenderAlias = regexp.MustCompile(`^blender[A-Za-z0-9_-]{1,57}$`)
var legacyBlenderSession = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)
var legacyBlenderOperation = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const blenderSignalLimit = 256 << 10

// The caller must hold the original AppCenter workspace and revalidate its
// binding. These Go-only methods never broaden Client.Send's public allowlist.
func legacyBlenderRoute(method, target string, body []byte) (string, int64, error) {
	if len(target) > 2048 || !safePath.MatchString(target) || len(body) > 16<<10 {
		return "", 0, ErrInvalid
	}
	parts := strings.Split(strings.TrimPrefix(target, "/"), "/")
	for i, part := range parts {
		if part == "." || part == ".." || part == "" && i != len(parts)-1 {
			return "", 0, ErrInvalid
		}
	}
	if method == "GET" && len(body) != 0 || method != "GET" && method != "POST" {
		return "", 0, ErrInvalid
	}
	if method == "POST" {
		value, err := strictjson.ReadValue(bytes.NewReader(body), 16<<10)
		value = bytes.TrimSpace(value)
		if err != nil || len(value) == 0 || value[0] != '{' {
			return "", 0, ErrInvalid
		}
	}
	if len(parts) == 2 && parts[0] == "mcp" && legacyBlenderAlias.MatchString(parts[1]) && method == "POST" {
		var call struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		}
		if strictjson.Decode(bytes.NewReader(body), 16<<10, &call) != nil || call.JSONRPC != "2.0" || call.Method != "tools/call" || len(call.ID) == 0 || bytes.Equal(call.ID, []byte("null")) || len(call.Params.Arguments) == 0 || call.Params.Arguments[0] != '{' {
			return "", 0, ErrInvalid
		}
		var requestID string
		if json.Unmarshal(call.ID, &requestID) != nil || requestID == "" || len(requestID) > 128 {
			return "", 0, ErrInvalid
		}
		switch call.Params.Name {
		case "session.open", "scene.get", "project.save", "operation.get", "session.close":
			return "application/json", 4 << 20, nil
		}
	}
	if strings.HasPrefix(target, "/studio/apps/blender/") {
		p := parts[3:]
		if len(p) == 1 {
			if method == "GET" && (p[0] == "instances" || p[0] == "instance-options") || method == "POST" && (p[0] == "instances" || p[0] == "instance-projects" || p[0] == "instance-source" || p[0] == "instance-project-create") {
				return "application/json", 4 << 20, nil
			}
		}
		if len(p) == 2 && (method == "GET" && p[0] == "instance-operations" && legacyBlenderOperation.MatchString(p[1]) || method == "POST" && legacyBlenderAlias.MatchString(p[0]) && (p[1] == "open" || p[1] == "close")) {
			return "application/json", 4 << 20, nil
		}
		if len(p) == 3 && method == "POST" && p[0] == "instances" && legacyBlenderAlias.MatchString(p[1]) && (p[2] == "start" || p[2] == "stop" || p[2] == "destroy") {
			return "application/json", 4 << 20, nil
		}
	}
	if len(parts) >= 5 && parts[0] == "apps" && parts[1] == "blender" && legacyBlenderAlias.MatchString(parts[2]) && legacyBlenderSession.MatchString(parts[3]) && method == "GET" {
		name := strings.Join(parts[4:], "/")
		if name == "settings" || name == "turn" {
			return "application/json", 4 << 20, nil
		}
		if name == "" {
			return "text/html", 16 << 20, nil
		}
		kind := map[string]string{".html": "text/html", ".js": "text/javascript", ".css": "text/css", ".svg": "image/svg+xml", ".png": "image/png", ".ico": "image/vnd.microsoft.icon", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf", ".json": "application/json"}[path.Ext(name)]
		if kind != "" {
			limit := int64(16 << 20)
			if kind == "application/json" {
				limit = 4 << 20
			}
			return kind, limit, nil
		}
	}
	return "", 0, ErrInvalid
}

// beginBlenderTransfer registers lifetime cancellation with the captured login.
// resetLocked cancels it on logout, new login, or client disposal.
func (s *Session) beginBlenderTransfer(ctx context.Context, timeout time.Duration) (context.Context, *http.Client, func(), error) {
	if ctx.Err() != nil {
		return nil, nil, nil, ErrUnavailable
	}
	c := s.client
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.authenticated || c.epoch != s.epoch {
		return nil, nil, nil, ErrChanged
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	if c.transfers == nil {
		c.transfers = map[uint64]context.CancelFunc{}
	}
	c.nextTransfer++
	id := c.nextTransfer
	c.transfers[id] = cancel
	var once sync.Once
	finish := func() { once.Do(func() { cancel(); c.mu.Lock(); delete(c.transfers, id); c.mu.Unlock() }) }
	client := &http.Client{Transport: c.transport, Jar: c.jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return requestCtx, client, finish, nil
}

func (s *Session) blenderUnauthorized() {
	c := s.client
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch == s.epoch {
		c.resetLocked()
	}
}

// LegacyBlender performs one bounded request without retrying or changing bytes.
// It exposes no cookie, remote CSP, arbitrary address, or HTTP transport object.
func (s *Session) LegacyBlender(ctx context.Context, method, target string, body []byte) (Response, error) {
	kind, limit, err := legacyBlenderRoute(method, target, body)
	if err != nil {
		return Response{}, err
	}
	requestCtx, client, finish, err := s.beginBlenderTransfer(ctx, 70*time.Second)
	if err != nil {
		return Response{}, err
	}
	defer finish()
	req, err := http.NewRequestWithContext(requestCtx, method, s.client.base+target, bytes.NewReader(body))
	if err != nil {
		return Response{}, ErrInvalid
	}
	req.Header.Set("Origin", s.client.base)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Accept", kind)
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		if !s.valid() {
			return Response{}, ErrChanged
		}
		return Response{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if !s.valid() {
		return Response{}, ErrChanged
	}
	if requestCtx.Err() != nil {
		return Response{}, ErrUnavailable
	}
	if resp.StatusCode == 401 {
		s.blenderUnauthorized()
		return Response{}, ErrDenied
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return Response{}, ErrDenied
	}
	if resp.StatusCode < 200 || resp.StatusCode > 599 || resp.StatusCode == 206 || resp.Header.Get("Content-Encoding") != "" {
		return Response{}, ErrInvalid
	}
	if resp.StatusCode >= 400 {
		limit = 4 << 20
	}
	if resp.ContentLength > limit {
		return Response{}, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if !s.valid() {
		return Response{}, ErrChanged
	}
	if requestCtx.Err() != nil {
		return Response{}, ErrUnavailable
	}
	if err != nil {
		return Response{}, ErrUnavailable
	}
	if int64(len(data)) > limit || resp.ContentLength >= 0 && resp.ContentLength != int64(len(data)) {
		return Response{}, ErrInvalid
	}
	if resp.StatusCode == 204 {
		if len(data) != 0 {
			return Response{}, ErrInvalid
		}
		return Response{Status: 204, MIME: "application/json"}, nil
	}
	if resp.StatusCode >= 400 && len(data) == 0 {
		return Response{Status: resp.StatusCode, MIME: "application/json", Data: []byte(`{"code":"LEGACY_UPSTREAM_ERROR"}`)}, nil
	}
	actual, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode >= 400 {
		kind = "application/json"
	}
	if err != nil || actual != kind && !(kind == "text/javascript" && actual == "application/javascript") && !(kind == "image/vnd.microsoft.icon" && actual == "image/x-icon") {
		return Response{}, ErrInvalid
	}
	if kind == "application/json" {
		if _, err := strictjson.ReadValue(bytes.NewReader(data), limit); err != nil {
			return Response{}, ErrInvalid
		}
	}
	return Response{Status: resp.StatusCode, MIME: kind, Date: resp.Header.Get("Date"), Data: data}, nil
}

// BlenderSignal is a Go-owned text connection. A local workspace must close it
// when its original binding is revoked; gateway lease checks remain authoritative.
type BlenderSignal struct {
	conn    *websocket.Conn
	session *Session
	ctx     context.Context
	finish  func()
	stop    func() bool
	once    sync.Once
}

func (s *Session) OpenBlenderSignal(ctx context.Context, alias, editingSession string) (*BlenderSignal, error) {
	if !legacyBlenderAlias.MatchString(alias) || !legacyBlenderSession.MatchString(editingSession) {
		return nil, ErrInvalid
	}
	requestCtx, client, finish, err := s.beginBlenderTransfer(ctx, 15*time.Minute)
	if err != nil {
		return nil, err
	}
	target := s.client.base + "/apps/blender/" + alias + "/" + editingSession + "/webrtc/signalling/"
	dialCtx, cancelDial := context.WithTimeout(requestCtx, 15*time.Second)
	conn, resp, err := websocket.Dial(dialCtx, target, &websocket.DialOptions{HTTPClient: client, HTTPHeader: http.Header{"Origin": {s.client.base}}, CompressionMode: websocket.CompressionDisabled})
	cancelDial()
	if err != nil {
		finish()
		if !s.valid() {
			return nil, ErrChanged
		}
		if resp != nil && resp.StatusCode == 401 {
			s.blenderUnauthorized()
			return nil, ErrDenied
		}
		return nil, ErrUnavailable
	}
	if !s.valid() || requestCtx.Err() != nil || conn.Subprotocol() != "" {
		conn.CloseNow()
		finish()
		if !s.valid() {
			return nil, ErrChanged
		}
		return nil, ErrUnavailable
	}
	conn.SetReadLimit(blenderSignalLimit)
	out := &BlenderSignal{conn: conn, session: s, ctx: requestCtx, finish: finish}
	out.stop = context.AfterFunc(requestCtx, func() { conn.CloseNow(); finish() })
	return out, nil
}

func (s *BlenderSignal) Close() {
	s.once.Do(func() { s.stop(); s.conn.CloseNow(); s.finish() })
}

func (s *BlenderSignal) Read(ctx context.Context) ([]byte, error) {
	if !s.session.valid() {
		s.Close()
		return nil, ErrChanged
	}
	if s.ctx.Err() != nil || ctx.Err() != nil {
		s.Close()
		return nil, ErrUnavailable
	}
	kind, data, err := s.conn.Read(ctx)
	if !s.session.valid() {
		s.Close()
		return nil, ErrChanged
	}
	if err != nil || s.ctx.Err() != nil {
		s.Close()
		return nil, ErrUnavailable
	}
	if kind != websocket.MessageText || len(data) > blenderSignalLimit || !utf8.Valid(data) {
		s.Close()
		return nil, ErrInvalid
	}
	return data, nil
}

func (s *BlenderSignal) Write(ctx context.Context, data []byte) error {
	if !s.session.valid() {
		s.Close()
		return ErrChanged
	}
	if len(data) == 0 || len(data) > blenderSignalLimit || !utf8.Valid(data) {
		s.Close()
		return ErrInvalid
	}
	if s.ctx.Err() != nil || ctx.Err() != nil {
		s.Close()
		return ErrUnavailable
	}
	err := s.conn.Write(ctx, websocket.MessageText, data)
	if !s.session.valid() {
		s.Close()
		return ErrChanged
	}
	if err != nil || s.ctx.Err() != nil {
		s.Close()
		return ErrUnavailable
	}
	return nil
}
