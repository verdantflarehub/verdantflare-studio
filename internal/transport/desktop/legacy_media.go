package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
	"golang.org/x/net/html"
)

const legacyWorkspacePrefix = "/studio/legacy-media"

var legacyBindingID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

type legacyMediaSession interface {
	Active() bool
	Send(context.Context, string, string, []byte) (studioremote.Response, error)
	LegacyMedia(context.Context, string, string, string, []byte, string, string) (*http.Response, error)
}

type legacyMediaLease struct {
	session legacyMediaSession
	request []byte
	opened  appcenter.LegacyOpened
	expires time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	active  chan struct{}
}

type desktopLegacyMedia struct {
	mu     sync.Mutex
	leases map[string]*legacyMediaLease
	bind   func() (legacyMediaSession, error)
	now    func() time.Time
}

func newDesktopLegacyMedia(s *Service) *desktopLegacyMedia {
	return &desktopLegacyMedia{leases: map[string]*legacyMediaLease{}, now: time.Now, bind: func() (legacyMediaSession, error) {
		if s == nil || s.remote == nil {
			return nil, studioremote.ErrUnavailable
		}
		return s.remote.Session()
	}}
}

func localLegacyOrigin(r *http.Request) string {
	scheme := r.URL.Scheme
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	if scheme != "http" && scheme != "https" {
		return ""
	}
	u, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || u.Host != r.Host || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	ip := net.ParseIP(u.Hostname())
	if u.Hostname() != "wails.localhost" && u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return ""
	}
	return u.String()
}

func legacyLocalRequest(r *http.Request, origin string, static bool) bool {
	if origin == "" || r.URL.RawPath != "" || strings.Contains(r.URL.EscapedPath(), "%") || r.URL.ForceQuery {
		return false
	}
	for _, key := range []string{"Cookie", "Authorization", "Content-Encoding"} {
		if len(r.Header.Values(key)) != 0 {
			return false
		}
	}
	origins, sites := r.Header.Values("Origin"), r.Header.Values("Sec-Fetch-Site")
	if len(origins) > 1 || len(sites) > 1 {
		return false
	}
	if static && r.Method == "GET" {
		return len(origins) == 0 || origins[0] == origin || origins[0] == "null"
	}
	if len(origins) == 1 && origins[0] != origin {
		return false
	}
	return len(sites) == 0 || sites[0] == "same-origin" || sites[0] == "none"
}

func (b *desktopLegacyMedia) pruneLocked() {
	for id, lease := range b.leases {
		if !b.now().Before(lease.expires) || !lease.session.Active() {
			lease.cancel()
			delete(b.leases, id)
		}
	}
}

func (b *desktopLegacyMedia) remove(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if lease := b.leases[id]; lease != nil {
		lease.cancel()
		delete(b.leases, id)
	}
}

func decodeLegacyOpened(data []byte, expected appcenter.OpenRequest) (appcenter.LegacyOpened, error) {
	var out appcenter.LegacyOpened
	if strictjson.Decode(bytes.NewReader(data), 16<<10, &out) != nil || out.SchemaVersion != 1 || out.Transport != "legacy" || out.AdapterID != expected.EntryID || out.BindingRevision != expected.ExpectedBindingRevision || !legacyBindingID.MatchString(out.BindingID) {
		return out, studioremote.ErrInvalid
	}
	adapter, route, err := appcenter.LegacyRoute(out.Route)
	if err != nil || adapter != out.AdapterID || route != out.Route {
		return out, studioremote.ErrInvalid
	}
	return out, nil
}

func (b *desktopLegacyMedia) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	fail := func(status int, code string) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
	}
	failRemote := func(err error) {
		out := remoteFailure(err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(out.Status)
		_, _ = w.Write(out.Data)
	}
	origin := localLegacyOrigin(r)
	if r.URL.Path == legacyWorkspacePrefix {
		if r.Method != "POST" || r.URL.RawQuery != "" || !legacyLocalRequest(r, origin, false) {
			fail(400, "INVALID_LEGACY_WORKSPACE")
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		var request appcenter.OpenRequest
		if len(r.Header.Values("Content-Type")) != 1 || err != nil || kind != "application/json" || appcenter.Decode(r.Body, "open", 4<<10, &request) != nil {
			fail(400, "INVALID_APPCENTER_REQUEST")
			return
		}
		if request.EntryID != "image-iframe-v1" && request.EntryID != "video-iframe-v1" {
			fail(409, "HOST_UNSUPPORTED")
			return
		}
		b.mu.Lock()
		b.pruneLocked()
		full := len(b.leases) >= 16
		b.mu.Unlock()
		if full {
			fail(429, "LEGACY_WORKSPACE_LIMIT")
			return
		}
		session, err := b.bind()
		if err != nil {
			failRemote(err)
			return
		}
		body, _ := json.Marshal(request)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		response, err := session.Send(ctx, "POST", "/studio/appcenter/v1/open", body)
		if err != nil {
			failRemote(err)
			return
		}
		if response.Status != 200 {
			fail(response.Status, "APPCENTER_OPEN_UNAVAILABLE")
			return
		}
		opened, err := decodeLegacyOpened(response.Data, request)
		if err != nil {
			fail(502, "APPCENTER_INVALID_OPEN")
			return
		}
		if ctx.Err() != nil {
			fail(503, "REQUEST_CANCELLED")
			return
		}
		leaseCtx, leaseCancel := context.WithCancel(context.Background())
		lease := &legacyMediaLease{session: session, request: body, opened: opened, expires: b.now().Add(30 * time.Minute), ctx: leaseCtx, cancel: leaseCancel, active: make(chan struct{}, 32)}
		b.mu.Lock()
		b.pruneLocked()
		if len(b.leases) >= 16 {
			b.mu.Unlock()
			leaseCancel()
			fail(429, "LEGACY_WORKSPACE_LIMIT")
			return
		}
		id := uuid.NewString()
		b.leases[id] = lease
		b.mu.Unlock()
		w.Header().Set("X-Studio-Legacy-Workspace", id)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(response.Data)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, legacyWorkspacePrefix+"/"), "/", 2)
	id, err := uuid.Parse(parts[0])
	if err != nil || id.Version() != 4 || id.String() != parts[0] {
		fail(404, "LEGACY_WORKSPACE_GONE")
		return
	}
	b.mu.Lock()
	b.pruneLocked()
	lease := b.leases[id.String()]
	b.mu.Unlock()
	if lease == nil {
		fail(410, "LEGACY_WORKSPACE_GONE")
		return
	}
	if len(parts) == 1 {
		if r.Method != "DELETE" || r.URL.RawQuery != "" || !legacyLocalRequest(r, origin, false) || r.ContentLength != 0 {
			fail(400, "INVALID_LEGACY_WORKSPACE")
			return
		}
		b.remove(id.String())
		w.WriteHeader(204)
		return
	}
	target := "/" + parts[1]
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	static := studioremote.LegacyMediaStatic(lease.opened.AdapterID, target)
	if !legacyLocalRequest(r, origin, static) {
		fail(403, "PERMISSION_DENIED")
		return
	}
	select {
	case lease.active <- struct{}{}:
		defer func() { <-lease.active }()
	default:
		fail(429, "LEGACY_WORKSPACE_LIMIT")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(lease.ctx, cancel)
	defer stop()
	defer cancel()
	if lease.ctx.Err() != nil {
		fail(410, "LEGACY_WORKSPACE_GONE")
		return
	}
	response, err := lease.session.Send(ctx, "POST", "/studio/appcenter/v1/open", lease.request)
	if err != nil {
		b.remove(id.String())
		failRemote(err)
		return
	}
	var expected appcenter.OpenRequest
	_ = json.Unmarshal(lease.request, &expected)
	opened, checkErr := decodeLegacyOpened(response.Data, expected)
	if response.Status != 200 || checkErr != nil || opened != lease.opened {
		b.remove(id.String())
		fail(409, "APPCENTER_BINDING_CHANGED")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 || len(r.Header.Values("Content-Type")) > 1 || len(r.Header.Values("Range")) > 1 {
		fail(400, "INVALID_LEGACY_REQUEST")
		return
	}
	out, err := lease.session.LegacyMedia(ctx, lease.opened.AdapterID, r.Method, target, body, r.Header.Get("Content-Type"), r.Header.Get("Range"))
	if err != nil {
		failRemote(err)
		return
	}
	defer out.Body.Close()
	// Wails Windows does not stream AssetHandler responses and does not recover
	// net/http's ErrAbortHandler. Verify one bounded chunk before writing headers.
	data, readErr := io.ReadAll(io.LimitReader(out.Body, (4<<20)+1))
	if readErr != nil || len(data) > 4<<20 {
		fail(502, "INVALID_LEGACY_RESPONSE")
		return
	}
	if !lease.session.Active() || ctx.Err() != nil {
		fail(409, "STUDIO_SESSION_CHANGED")
		return
	}
	b.mu.Lock()
	if b.leases[id.String()] == lease {
		lease.expires = b.now().Add(30 * time.Minute)
	}
	b.mu.Unlock()
	w.Header().Set("Content-Type", out.Header.Get("Content-Type"))
	if static {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	}
	if out.Header.Get("Content-Type") == "text/html" && out.StatusCode == 200 {
		data, err = rewriteLegacyMediaHTML(data, lease.opened.AdapterID, legacyWorkspacePrefix+"/"+id.String())
		if err != nil || len(data) > 4<<20 {
			fail(502, "INVALID_LEGACY_RESPONSE")
			return
		}
		if !lease.session.Active() || ctx.Err() != nil {
			fail(409, "STUDIO_SESSION_CHANGED")
			return
		}
		inline := ""
		images := "blob: data:"
		if lease.opened.AdapterID == "image-iframe-v1" {
			inline = " 'unsafe-inline'"
			images += " https://images.unsplash.com"
		}
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src "+origin+inline+"; style-src "+origin+" 'unsafe-inline'; img-src "+images+"; media-src blob:; connect-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(200)
		_, _ = w.Write(data)
		return
	}
	for _, key := range []string{"Content-Range", "Accept-Ranges", "ETag"} {
		if value := out.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(out.StatusCode)
	_, _ = w.Write(data)
}

// Only registered script/style URLs are rewritten; script bodies, business
// links and all other markup retain their original bytes.
func rewriteLegacyMediaHTML(data []byte, adapter, prefix string) ([]byte, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	var output bytes.Buffer
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() == io.EOF {
				return output.Bytes(), nil
			}
			return nil, z.Err()
		}
		raw := append([]byte(nil), z.Raw()...)
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			output.Write(raw)
			continue
		}
		token := z.Token()
		changed := false
		if token.Data == "script" || token.Data == "link" {
			for i, attr := range token.Attr {
				if token.Data == "script" && attr.Key != "src" || token.Data == "link" && attr.Key != "href" {
					continue
				}
				path := strings.TrimPrefix(attr.Val, ".")
				name := "video"
				if adapter == "image-iframe-v1" {
					name = "image"
				}
				for _, oldPrefix := range []string{"/apps/" + name, "/studio/apps/" + name} {
					path = strings.TrimPrefix(path, oldPrefix)
				}
				if studioremote.LegacyMediaStatic(adapter, path) {
					token.Attr[i].Val = prefix + path
					changed = true
				}
			}
		}
		if changed {
			output.WriteString(token.String())
		} else {
			output.Write(raw)
		}
	}
}
