package studioremote

import (
	"context"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

// Session binds a local working copy to the login that opened it. It exposes no
// credential and cannot silently acquire another user's newly logged-in session.
type Session struct {
	client *Client
	epoch  uint64
}

func (c *Client) Session() (*Session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.authenticated {
		return nil, ErrDenied
	}
	return &Session{client: c, epoch: c.epoch}, nil
}

func (s *Session) Send(ctx context.Context, method, path string, body []byte) (Response, error) {
	return s.client.send(ctx, method, path, body, &s.epoch)
}

func (s *Session) valid() bool {
	c := s.client
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticated && c.epoch == s.epoch
}

// Active lets Go-owned workspace registries retire handles after login changes.
// It is a lifetime check, never a substitute for remote authorization.
func (s *Session) Active() bool { return s.valid() }

// Origin returns only the configured gateway origin of this still-active login.
// It is display metadata, never an authority to forward arbitrary URLs.
func (s *Session) Origin() string {
	c := s.client
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.authenticated || c.epoch != s.epoch {
		return ""
	}
	return c.base
}

type sessionBody struct {
	reader  io.Reader
	closer  io.Closer
	session *Session
	finish  func()
}

func (b *sessionBody) Read(p []byte) (int, error) {
	if !b.session.valid() {
		return 0, ErrChanged
	}
	n, err := b.reader.Read(p)
	if !b.session.valid() {
		clear(p[:n])
		return 0, ErrChanged
	}
	return n, err
}

func (b *sessionBody) Close() error {
	if b.finish != nil {
		b.finish()
	}
	if b.closer != nil {
		return b.closer.Close()
	}
	return nil
}

// Transfer is a Go-only Artifact transport, separate from the WebView JSON
// bridge. The caller owns its upload source and closes the returned response.
func (s *Session) Transfer(ctx context.Context, method, path, query string, body io.Reader, length, maxBytes int64) (*http.Response, error) {
	if _, err := mcp.TransferScope(method, path, query); err != nil || maxBytes <= 0 || maxBytes > 1<<50 || length < 0 || length > maxBytes || (method == "GET" && (body != nil || length != 0)) || (method == "PUT" && body == nil) {
		return nil, ErrInvalid
	}
	return s.stream(ctx, method, path, query, body, length, maxBytes)
}

// Content accepts only an opaque Dashboard content handle issued by Studio.
func (s *Session) Content(ctx context.Context, path string) (*http.Response, error) {
	if !contentHandlePath.MatchString(path) {
		return nil, ErrInvalid
	}
	return s.stream(ctx, "GET", path, "", nil, 0, 1<<30)
}

func (s *Session) Media(ctx context.Context, method, path string, headers http.Header) (*http.Response, error) {
	if !stableMediaPath.MatchString(path) || (method != "GET" && method != "HEAD") {
		return nil, ErrInvalid
	}
	selected := http.Header{}
	for _, key := range []string{"Range", "If-Range", "If-None-Match"} {
		if len(headers.Values(key)) > 1 || len(headers.Get(key)) > 1024 {
			return nil, ErrInvalid
		}
		if value := headers.Get(key); value != "" {
			selected.Set(key, value)
		}
	}
	return s.streamHeaders(ctx, method, path, "", nil, 0, 1<<30, selected)
}
func (s *Session) stream(ctx context.Context, method, path, query string, body io.Reader, length, maxBytes int64) (*http.Response, error) {
	return s.streamHeaders(ctx, method, path, query, body, length, maxBytes, nil)
}
func (s *Session) streamHeaders(ctx context.Context, method, path, query string, body io.Reader, length, maxBytes int64, headers http.Header) (*http.Response, error) {
	c := s.client
	c.mu.Lock()
	if c.epoch != s.epoch || !c.authenticated {
		c.mu.Unlock()
		return nil, ErrChanged
	}
	transferCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	if c.transfers == nil {
		c.transfers = map[uint64]context.CancelFunc{}
	}
	c.nextTransfer++
	key, jar := c.nextTransfer, c.jar
	c.transfers[key] = cancel
	c.mu.Unlock()
	var once sync.Once
	finish := func() { once.Do(func() { cancel(); c.mu.Lock(); delete(c.transfers, key); c.mu.Unlock() }) }
	var source io.Reader
	if body != nil {
		source = &sessionBody{reader: body, session: s}
	}
	target := c.base + path
	if query != "" {
		target += "?" + query
	}
	req, err := http.NewRequestWithContext(transferCtx, method, target, source)
	if err != nil {
		finish()
		return nil, ErrInvalid
	}
	for key, values := range headers {
		req.Header[key] = values
	}
	req.ContentLength = length
	req.Header.Set("Origin", c.base)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	client := &http.Client{Transport: c.transport, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		finish()
		if !s.valid() {
			return nil, ErrChanged
		}
		return nil, ErrUnavailable
	}
	if !s.valid() {
		resp.Body.Close()
		finish()
		return nil, ErrChanged
	}
	if resp.StatusCode == 401 {
		c.mu.Lock()
		if c.epoch == s.epoch {
			c.resetLocked()
		}
		c.mu.Unlock()
		resp.Body.Close()
		finish()
		return nil, ErrDenied
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 && !(stableMediaPath.MatchString(path) && resp.StatusCode == 304) {
		resp.Body.Close()
		finish()
		return nil, ErrDenied
	}
	if resp.StatusCode == 200 || resp.StatusCode == 206 {
		mediaType, _, mimeErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		invalid := mimeErr != nil || resp.Header.Get("Content-Encoding") != "" || resp.ContentLength < 0 || resp.ContentLength > maxBytes
		if method == "PUT" {
			invalid = mimeErr != nil || mediaType != "application/json" || resp.Header.Get("Content-Encoding") != "" || resp.ContentLength > 4096
		}
		if invalid {
			resp.Body.Close()
			finish()
			return nil, ErrInvalid
		}
	}
	resp.Body = &sessionBody{reader: resp.Body, closer: resp.Body, session: s, finish: finish}
	return resp, nil
}
