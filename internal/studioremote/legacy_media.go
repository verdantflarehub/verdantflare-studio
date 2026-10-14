package studioremote

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	"github.com/verdantflarehub/verdantflare-studio/internal/videotitle"
)

const legacyJSONLimit = 4 << 20
const legacyMediaLimit = 1 << 30

var legacyImageRead = regexp.MustCompile(`^/(api/(tasks|tasks/[a-zA-Z0-9_-]+|tasks/stats|mcp/status)|artifacts/[a-zA-Z0-9_-]+/content)$`)
var legacyVideoRead = regexp.MustCompile(`^/api/(dashboard|capabilities|model-catalog|mcp/status|models|models/[a-z0-9-]+/instances(/[a-zA-Z0-9._-]+(/gpus/[a-zA-Z0-9._:-]+)?)?|tasks/[a-zA-Z0-9_-]+)$`)
var legacyVideoWrite = regexp.MustCompile(`^/api/(tasks|tasks/[a-zA-Z0-9_-]+/(result|title)|artifacts/import)$`)
var legacyVideoTask = regexp.MustCompile(`^/dashboard/tasks/[a-zA-Z0-9_-]+$`)
var legacyBinaryPath = regexp.MustCompile(`^/(artifacts/[a-zA-Z0-9_-]+/content|api/tasks/[a-zA-Z0-9_-]+/(thumbnail|content))$`)
var legacyRange = regexp.MustCompile(`^bytes=([0-9]+)-([0-9]*)$|^bytes=-([0-9]+)$`)
var legacyContentRange = regexp.MustCompile(`^bytes ([0-9]+)-([0-9]+)/([0-9]+)$`)

type legacyMediaRoute struct {
	path, query, kind string
}

// LegacyMediaStatic permits only script/style subresources of an already
// authorized opaque-origin iframe, never its JSON or binary business data.
func LegacyMediaStatic(adapter, target string) bool {
	route, err := legacyRoute(adapter, "GET", target)
	return err == nil && (route.kind == "text/javascript" || route.kind == "text/css")
}

// Only the existing host-owned media adapters may select these routes. This is
// not the JSON bridge allowlist and does not accept a caller-provided upstream.
func legacyRoute(adapter, method, target string) (legacyMediaRoute, error) {
	var out legacyMediaRoute
	if len(target) > 2048 || strings.ContainsAny(target, "\\#\x00\r\n\t\x7f") {
		return out, ErrInvalid
	}
	u, err := url.ParseRequestURI(target)
	if err != nil || u.Scheme != "" || u.Host != "" || u.RawPath != "" || u.ForceQuery || !strings.HasPrefix(u.Path, "/") || strings.Contains(u.EscapedPath(), "%") {
		return out, ErrInvalid
	}
	for _, part := range strings.Split(u.Path[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return out, ErrInvalid
		}
	}
	prefix, kind := "", ""
	switch adapter {
	case "image-iframe-v1":
		prefix = "/studio/apps/image"
		if method == "GET" {
			switch u.Path {
			case "/dashboard":
				kind = "text/html"
			case "/dashboard/static/dashboard.js":
				kind = "text/javascript"
			case "/dashboard/static/dashboard.css":
				kind = "text/css"
			default:
				if legacyImageRead.MatchString(u.Path) {
					kind = "application/json"
				}
			}
		} else if method == "POST" && (u.Path == "/api/tasks" || u.Path == "/api/artifacts/upload") {
			kind = "application/json"
		}
	case "video-iframe-v1":
		prefix = "/studio/apps/video"
		if method == "GET" {
			switch u.Path {
			case "/dashboard/static/model-catalog.css", "/dashboard/frontend/model-catalog.css":
				kind = "text/css"
			case "/dashboard/static/model-catalog.js", "/dashboard/frontend/model-catalog.js":
				kind = "text/javascript"
			case "/dashboard":
				kind = "text/html"
			case "/dashboard/static/dashboard.css", "/dashboard/static/studio-theme.css", "/dashboard/frontend/dashboard.css", "/dashboard/frontend/studio-theme.css":
				kind = "text/css"
			case "/dashboard/static/dashboard.js", "/dashboard/static/resources.js", "/dashboard/static/task-detail.js", "/dashboard/static/studio-embed.js",
				"/dashboard/frontend/dashboard.js", "/dashboard/frontend/resources.js", "/dashboard/frontend/task-detail.js", "/dashboard/frontend/studio-embed.js":
				kind = "text/javascript"
			default:
				if legacyVideoTask.MatchString(u.Path) {
					kind = "text/html"
				} else if legacyVideoRead.MatchString(u.Path) {
					kind = "application/json"
				}
			}
		} else if method == "POST" && legacyVideoWrite.MatchString(u.Path) {
			kind = "application/json"
		}
	default:
		return out, ErrInvalid
	}
	if method == "GET" && legacyBinaryPath.MatchString(u.Path) && (adapter == "video-iframe-v1" || strings.HasPrefix(u.Path, "/artifacts/")) {
		kind = "binary"
	}
	if kind == "" {
		return out, ErrInvalid
	}
	allowedQuery := map[string]bool{}
	if kind == "text/html" {
		for _, key := range []string{"embed", "theme", "view"} {
			allowedQuery[key] = true
		}
	} else if method == "GET" && u.Path == "/api/tasks" && adapter == "image-iframe-v1" {
		for _, key := range []string{"limit", "offset", "status", "engine", "project_id"} {
			allowedQuery[key] = true
		}
	} else if method == "GET" && u.Path == "/api/dashboard" {
		for _, key := range []string{"page", "page_size", "status", "service", "project_id", "q"} {
			allowedQuery[key] = true
		}
	} else if method == "GET" && strings.Contains(u.Path, "/gpus/") {
		allowedQuery["window"] = true
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return out, ErrInvalid
	}
	for key, values := range query {
		if !allowedQuery[key] || len(values) != 1 || len(values[0]) > 512 {
			return out, ErrInvalid
		}
		for _, char := range values[0] {
			if char < 32 || char == 127 || char == '\\' {
				return out, ErrInvalid
			}
		}
	}
	return legacyMediaRoute{prefix + u.Path, query.Encode(), kind}, nil
}

// LegacyMedia is Go-only. The caller must retain the Session captured when the
// bound legacy workspace opened, and close every returned body. It cannot
// acquire a new login for a stale page. No browser credentials are accepted.
func (s *Session) LegacyMedia(ctx context.Context, adapter, method, target string, body []byte, contentType, byteRange string) (*http.Response, error) {
	route, err := legacyRoute(adapter, method, target)
	if err != nil || len(body) > 1<<20 || (method == "GET" && (len(body) != 0 || contentType != "")) {
		return nil, ErrInvalid
	}
	if byteRange != "" && (route.kind != "binary" || !validLegacyRange(byteRange)) {
		return nil, ErrInvalid
	}
	if method == "POST" {
		if strings.HasSuffix(route.path, "/title") && !videotitle.ValidBody(body) {
			return nil, ErrInvalid
		}
		kind, params, err := mime.ParseMediaType(contentType)
		if adapter == "image-iframe-v1" && route.path == "/studio/apps/image/api/artifacts/upload" {
			if err != nil || kind != "multipart/form-data" || len(params) != 1 || params["boundary"] == "" || len(params["boundary"]) > 70 || len(body) == 0 {
				return nil, ErrInvalid
			}
		} else {
			if err != nil || kind != "application/json" || len(params) > 1 || len(params) == 1 && !strings.EqualFold(params["charset"], "utf-8") {
				return nil, ErrInvalid
			}
			// The original result endpoint accepts an empty POST.
			if len(body) != 0 {
				if _, err = strictjson.ReadValue(bytes.NewReader(body), 1<<20); err != nil {
					return nil, ErrInvalid
				}
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	c := s.client
	c.mu.Lock()
	if c.epoch != s.epoch || !c.authenticated {
		c.mu.Unlock()
		return nil, ErrChanged
	}
	requestCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	if c.transfers == nil {
		c.transfers = map[uint64]context.CancelFunc{}
	}
	c.nextTransfer++
	key, jar := c.nextTransfer, c.jar
	c.transfers[key] = cancel
	c.mu.Unlock()
	var once sync.Once
	finish := func() { once.Do(func() { cancel(); c.mu.Lock(); delete(c.transfers, key); c.mu.Unlock() }) }
	targetURL := c.base + route.path
	if route.query != "" {
		targetURL += "?" + route.query
	}
	req, err := http.NewRequestWithContext(requestCtx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		finish()
		return nil, ErrInvalid
	}
	req.Header.Set("Origin", c.base)
	req.Header.Set("Accept-Encoding", "identity")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	client := &http.Client{Transport: c.transport, Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		finish()
		if !s.valid() {
			return nil, ErrChanged
		}
		return nil, ErrUnavailable
	}
	fail := func(err error) (*http.Response, error) { resp.Body.Close(); finish(); return nil, err }
	if requestCtx.Err() != nil {
		return fail(ErrUnavailable)
	}
	if !s.valid() {
		return fail(ErrChanged)
	}
	if resp.StatusCode == 401 {
		c.mu.Lock()
		if c.epoch == s.epoch {
			c.resetLocked()
		}
		c.mu.Unlock()
		return fail(ErrDenied)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return fail(ErrDenied)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 599 || resp.Header.Get("Content-Encoding") != "" {
		return fail(ErrInvalid)
	}
	limit := int64(legacyJSONLimit)
	// Wails Windows buffers an AssetHandler response. Each media request is a
	// bounded chunk; the parent bridge assembles at most legacyMediaLimit bytes.
	if resp.ContentLength > limit {
		return fail(ErrInvalid)
	}
	kind, _, mimeErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if resp.StatusCode < 300 && resp.StatusCode != 204 {
		valid := kind == route.kind || route.kind == "text/javascript" && kind == "application/javascript"
		if route.kind == "binary" {
			valid = legacyMediaMIME(kind)
		}
		if mimeErr != nil || !valid {
			return fail(ErrInvalid)
		}
		if resp.StatusCode == 206 && (route.kind != "binary" || !validLegacyContentRange(resp, byteRange)) {
			return fail(ErrInvalid)
		}
	}
	// Never expose remote cookies, URLs, CSP, or transport objects to the local
	// handler. It must construct its own CSP for the trusted local host origin.
	out := &http.Response{StatusCode: resp.StatusCode, ContentLength: resp.ContentLength, Header: http.Header{}}
	out.Header.Set("Content-Type", kind)
	for _, name := range []string{"Content-Range", "Accept-Ranges", "ETag"} {
		if route.kind == "binary" && resp.StatusCode < 300 {
			out.Header.Set(name, resp.Header.Get(name))
		}
	}
	reader := &legacyBoundBody{ctx: requestCtx, body: resp.Body, session: s, remaining: limit, expected: resp.ContentLength, finish: finish}
	if route.kind == "application/json" || resp.StatusCode >= 400 || resp.StatusCode == 204 {
		data, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == 204 {
			if len(data) != 0 {
				return nil, ErrInvalid
			}
		} else if len(data) == 0 && resp.StatusCode >= 400 {
			data = []byte(`{"code":"LEGACY_UPSTREAM_ERROR"}`)
		} else if mimeErr != nil || kind != "application/json" {
			return nil, ErrInvalid
		} else if _, err = strictjson.ReadValue(bytes.NewReader(data), legacyJSONLimit); err != nil {
			return nil, ErrInvalid
		}
		out.Header.Set("Content-Type", "application/json")
		out.ContentLength = int64(len(data))
		out.Body = &legacyBoundBody{ctx: ctx, body: io.NopCloser(bytes.NewReader(data)), session: s, remaining: limit, expected: int64(len(data)), finish: func() {}}
	} else {
		out.Body = reader
	}
	return out, nil
}

func validLegacyRange(value string) bool {
	m := legacyRange.FindStringSubmatch(value)
	if m == nil {
		return false
	}
	if m[3] != "" {
		n, err := strconv.ParseInt(m[3], 10, 64)
		return err == nil && n > 0
	}
	start, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return false
	}
	if m[2] == "" {
		return true
	}
	end, err := strconv.ParseInt(m[2], 10, 64)
	return err == nil && end >= start
}

func validLegacyContentRange(resp *http.Response, requested string) bool {
	m := legacyContentRange.FindStringSubmatch(resp.Header.Get("Content-Range"))
	if m == nil || !validLegacyRange(requested) {
		return false
	}
	values := make([]int64, 3)
	for i := range values {
		var err error
		values[i], err = strconv.ParseInt(m[i+1], 10, 64)
		if err != nil {
			return false
		}
	}
	start, end, total := values[0], values[1], values[2]
	if end < start || total <= end || total > legacyMediaLimit || resp.ContentLength != end-start+1 {
		return false
	}
	rangeParts := legacyRange.FindStringSubmatch(requested)
	if rangeParts[3] != "" {
		n, _ := strconv.ParseInt(rangeParts[3], 10, 64)
		return end == total-1 && start == max(0, total-n)
	}
	first, _ := strconv.ParseInt(rangeParts[1], 10, 64)
	last := total - 1
	if rangeParts[2] != "" {
		last, _ = strconv.ParseInt(rangeParts[2], 10, 64)
		last = min(last, total-1)
	}
	return start == first && end == last
}

func legacyMediaMIME(kind string) bool {
	switch kind {
	case "image/png", "image/jpeg", "image/webp", "image/gif", "image/avif", "video/mp4", "video/webm", "audio/mpeg", "audio/wav", "audio/ogg", "application/octet-stream":
		return true
	}
	return false
}

type legacyBoundBody struct {
	ctx                       context.Context
	body                      io.ReadCloser
	session                   *Session
	remaining, expected, read int64
	finish                    func()
}

func (b *legacyBoundBody) Read(p []byte) (int, error) {
	if !b.session.valid() {
		return 0, ErrChanged
	}
	if b.ctx.Err() != nil {
		return 0, ErrUnavailable
	}
	if len(p) == 0 {
		return 0, nil
	}
	maximum := b.remaining
	if b.expected >= 0 {
		maximum = min(maximum, b.expected-b.read)
	}
	probe := maximum == 0
	if probe {
		p = make([]byte, 1)
	} else if int64(len(p)) > maximum {
		p = p[:maximum]
	}
	n, err := b.body.Read(p)
	if !b.session.valid() {
		clear(p[:n])
		return 0, ErrChanged
	}
	if b.ctx.Err() != nil {
		clear(p[:n])
		return 0, ErrUnavailable
	}
	if probe && n != 0 {
		return 0, ErrInvalid
	}
	b.remaining -= int64(n)
	b.read += int64(n)
	if err == io.EOF && b.expected >= 0 && b.read != b.expected {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func (b *legacyBoundBody) Close() error { b.finish(); return b.body.Close() }
