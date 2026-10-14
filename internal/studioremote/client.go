// Package studioremote holds a desktop user's Studio session outside the WebView.
package studioremote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var ErrUnavailable = errors.New("STUDIO_REMOTE_UNAVAILABLE")
var ErrDenied = errors.New("STUDIO_REMOTE_DENIED")
var ErrInvalid = errors.New("INVALID_STUDIO_REMOTE_REQUEST")
var ErrChanged = errors.New("STUDIO_REMOTE_SESSION_CHANGED")
var safePath = regexp.MustCompile(`^/[a-zA-Z0-9._/-]+$`)
var sessionPath = regexp.MustCompile(`^/studio/dashboards/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}(/requests|/instances/open|/assets/[a-zA-Z0-9._/-]+)?$`)
var workspacePath = regexp.MustCompile(`^/studio/instance-workspaces/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}(/requests|/assets/[a-zA-Z0-9._/-]+)?$`)
var publicInstanceAsset = regexp.MustCompile(`^/studio/instance-assets/[a-f0-9]{64}/[a-zA-Z0-9._/-]+$`)
var apiDetail = regexp.MustCompile(`^/api/(apps/[a-z0-9-]+|installations/[0-9a-f-]+|operations/[0-9a-f-]+|proxies/[0-9a-f-]+(/test)?)$`)
var contentCapabilityPath = regexp.MustCompile(`^/studio/dashboards/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/capabilities/artifact\.read$`)
var contentHandlePath = regexp.MustCompile(`^/studio/dashboards/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}/content/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

type Response struct {
	Status                int
	Data                  []byte
	MIME, RequestID, Date string
}
type Client struct {
	mu            sync.Mutex
	base          string
	url           *url.URL
	transport     http.RoundTripper
	jar           http.CookieJar
	epoch         uint64
	authenticated bool
	transfers     map[uint64]context.CancelFunc
	nextTransfer  uint64
}

func New(origin string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || strings.ContainsAny(origin, "\\ \t\r\n?#") {
		return nil, ErrInvalid
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, ErrInvalid
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	jar, _ := cookiejar.New(nil)
	return &Client{base: origin, url: u, transport: transport, jar: jar}, nil
}

func Allowed(method, path string) bool {
	if len(path) > 2048 || !safePath.MatchString(path) {
		return false
	}
	for _, part := range strings.Split(path[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	switch path {
	case "/studio/appcenter/v1/view":
		return method == "GET"
	case "/studio/appcenter/v1/open", "/studio/appcenter/v1/commands/prepare", "/studio/appcenter/v1/commands":
		return method == "POST"
	case "/api/login", "/api/logout", "/api/commands", "/api/resources/workspaces/release", "/studio/dashboards", "/studio/navigation/preferences", "/studio/preinstalled/reconcile", "/mcp":
		return method == "POST"
	case "/api/me", "/api/health", "/api/apps", "/api/installations", "/api/resources/gpu", "/api/resources/node", "/api/resources/summary", "/api/resources/workloads", "/api/resources/workspaces", "/studio/dashboard-installations", "/studio/navigation", "/studio/preinstalled":
		return method == "GET"
	case "/api/proxies":
		return method == "GET" || method == "POST"
	}
	if apiDetail.MatchString(path) {
		if strings.HasSuffix(path, "/test") {
			return method == "POST"
		}
		if strings.HasPrefix(path, "/api/proxies/") {
			return method == "GET" || method == "PUT" || method == "DELETE"
		}
		return method == "GET"
	}
	if contentCapabilityPath.MatchString(path) {
		return method == "POST"
	}
	if publicInstanceAsset.MatchString(path) {
		return method == "GET" && workspaceAssetMIME(path) != ""
	}
	if workspacePath.MatchString(path) {
		if strings.HasSuffix(path, "/requests") {
			return method == "POST"
		}
		if strings.Contains(path, "/assets/") {
			return method == "GET" && workspaceAssetMIME(path) != ""
		}
		return method == "GET" || method == "DELETE"
	}
	if sessionPath.MatchString(path) {
		if strings.HasSuffix(path, "/requests") || strings.HasSuffix(path, "/instances/open") {
			return method == "POST"
		}
		if strings.Contains(path, "/assets/") {
			return method == "GET" && (strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".mjs") || strings.HasSuffix(path, ".css"))
		}
		return method == "GET" || method == "DELETE"
	}
	return false
}

func workspaceAssetMIME(name string) string {
	return map[string]string{".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css",
		".json": "application/json", ".png": "image/png", ".svg": "image/svg+xml", ".jpg": "image/jpeg", ".webp": "image/webp",
		".ico": "image/vnd.microsoft.icon", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf", ".wasm": "application/wasm"}[path.Ext(name)]
}

func (c *Client) resetLocked() {
	for _, cancel := range c.transfers {
		cancel()
	}
	c.epoch++
	c.jar, _ = cookiejar.New(nil)
	c.authenticated = false
}
func (c *Client) Close() {
	c.mu.Lock()
	c.resetLocked()
	c.mu.Unlock()
	if t, ok := c.transport.(interface{ CloseIdleConnections() }); ok {
		t.CloseIdleConnections()
	}
}

// Send accepts host-approved paths only. It never forwards browser headers.
func (c *Client) Send(ctx context.Context, method, path string, body []byte) (Response, error) {
	return c.send(ctx, method, path, body, nil)
}

func (c *Client) send(ctx context.Context, method, path string, body []byte, expected *uint64) (Response, error) {
	if ctx.Err() != nil {
		return Response{}, ErrUnavailable
	}
	if !Allowed(method, path) || len(body) > (1<<20)+4096 || method == "GET" && len(body) != 0 {
		return Response{}, ErrInvalid
	}
	if len(body) > 0 {
		if _, err := strictjson.ReadValue(bytes.NewReader(body), (1<<20)+4096); err != nil {
			return Response{}, ErrInvalid
		}
	}
	login := path == "/api/login"
	logout := path == "/api/logout"
	c.mu.Lock()
	if ctx.Err() != nil {
		c.mu.Unlock()
		return Response{}, ErrUnavailable
	}
	if expected != nil && (*expected != c.epoch || login || logout) {
		c.mu.Unlock()
		return Response{}, ErrChanged
	}
	if login {
		c.resetLocked()
	}
	if !login && !c.authenticated {
		c.mu.Unlock()
		return Response{}, ErrDenied
	}
	epoch, jar := c.epoch, c.jar
	c.mu.Unlock()
	if logout {
		defer func() {
			c.mu.Lock()
			if c.epoch == epoch {
				c.resetLocked()
			}
			c.mu.Unlock()
		}()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return Response{}, ErrInvalid
	}
	req.Header.Set("Origin", c.base)
	req.Header.Set("Accept", "application/json")
	if method == "POST" || method == "PUT" || method == "PATCH" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := http.Client{Transport: c.transport, Jar: jar, Timeout: 70 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return Response{}, ErrDenied
	}
	workspaceAsset := publicInstanceAsset.MatchString(path) || (workspacePath.MatchString(path) && strings.Contains(path, "/assets/"))
	maximum := int64(4 << 20)
	if workspaceAsset {
		maximum = 16 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return Response{}, ErrUnavailable
	}
	mimeType, _, mimeErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode == 204 {
		if len(data) != 0 {
			return Response{}, ErrInvalid
		}
	} else if mimeErr != nil {
		return Response{}, ErrInvalid
	}
	asset := (strings.Contains(path, "/assets/") || publicInstanceAsset.MatchString(path)) && resp.StatusCode == 200
	if asset {
		expected := "text/javascript"
		if strings.HasSuffix(path, ".html") {
			expected = "text/html"
		}
		if strings.HasSuffix(path, ".css") {
			expected = "text/css"
		}
		if workspaceAsset {
			expected = workspaceAssetMIME(path)
		}
		if mimeType != expected {
			return Response{}, ErrInvalid
		}
	} else if resp.StatusCode != 204 {
		if mimeType != "application/json" {
			return Response{}, ErrInvalid
		}
		if _, err := strictjson.ReadValue(bytes.NewReader(data), 4<<20); err != nil {
			return Response{}, ErrInvalid
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil {
		return Response{}, ErrUnavailable
	}
	if epoch != c.epoch {
		return Response{}, ErrChanged
	}
	if login && resp.StatusCode == 201 {
		for _, cookie := range jar.Cookies(c.url) {
			if cookie.Name == "vf_studio_session" && cookie.Value != "" {
				c.authenticated = true
			}
		}
		if !c.authenticated {
			return Response{}, ErrDenied
		}
	}
	if resp.StatusCode == 401 {
		c.resetLocked()
	}
	return Response{Status: resp.StatusCode, Data: data, MIME: mimeType, RequestID: resp.Header.Get("X-Request-ID"), Date: resp.Header.Get("Date")}, nil
}
