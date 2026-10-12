package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
)

const blenderWorkspacePrefix = "/studio/legacy-blender"

var desktopBlenderAlias = regexp.MustCompile(`^blender[A-Za-z0-9_-]{1,57}$`)
var desktopBlenderEdit = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)

type blenderRemoteSession interface {
	Active() bool
	Origin() string
	Send(context.Context, string, string, []byte) (studioremote.Response, error)
	LegacyBlender(context.Context, string, string, []byte) (studioremote.Response, error)
}

type blenderWire interface {
	Read(context.Context) ([]byte, error)
	Write(context.Context, []byte) error
	Close()
}

type desktopBlenderEditLease struct {
	alias, id string
	opening   bool
	signal    *desktopBlenderSignal
}

type desktopBlenderSignal struct {
	id               string
	wire             blenderWire
	ctx              context.Context
	cancel           context.CancelFunc
	incoming         chan string
	reading, writing chan struct{}
}

type desktopBlenderLease struct {
	id      string
	session blenderRemoteSession
	request []byte
	opened  appcenter.LegacyOpened
	ctx     context.Context
	cancel  context.CancelFunc
	expires time.Time
	active  chan struct{}
	edits   map[string]*desktopBlenderEditLease
	opening int
}

type desktopLegacyBlender struct {
	mu                         sync.Mutex
	leases                     map[string]*desktopBlenderLease
	bind                       func() (blenderRemoteSession, error)
	dial                       func(context.Context, blenderRemoteSession, string, string) (blenderWire, error)
	now                        func() time.Time
	checkInterval, pollTimeout time.Duration
}

func newDesktopLegacyBlender(s *Service) *desktopLegacyBlender {
	return &desktopLegacyBlender{leases: map[string]*desktopBlenderLease{}, now: time.Now, checkInterval: 5 * time.Second, pollTimeout: 20 * time.Second,
		bind: func() (blenderRemoteSession, error) {
			if s == nil || s.remote == nil {
				return nil, studioremote.ErrUnavailable
			}
			return s.remote.Session()
		},
		dial: func(ctx context.Context, session blenderRemoteSession, alias, id string) (blenderWire, error) {
			return session.(*studioremote.Session).OpenBlenderSignal(ctx, alias, id)
		},
	}
}

func blenderJSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != 204 {
		return json.NewEncoder(w).Encode(value)
	}
	return nil
}
func blenderFailure(w http.ResponseWriter, status int, code string) {
	blenderJSON(w, status, map[string]string{"code": code})
}
func blenderDecode(r *http.Request, limit int64, value any) bool {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return len(r.Header.Values("Content-Type")) == 1 && err == nil && kind == "application/json" && strictjson.Decode(r.Body, limit, value) == nil
}

func (b *desktopLegacyBlender) remove(id string) {
	b.mu.Lock()
	lease := b.leases[id]
	delete(b.leases, id)
	var edits [][2]string
	if lease != nil {
		lease.cancel()
		for _, edit := range lease.edits {
			edits = append(edits, [2]string{edit.alias, edit.id})
		}
	}
	b.mu.Unlock()
	if lease == nil || len(edits) == 0 {
		return
	}
	releaseBlenderEdits(lease, edits)
}

func releaseBlenderEdits(lease *desktopBlenderLease, edits [][2]string) {
	// Cleanup may only release captured editing leases under the original entry.
	// A changed gateway binding or login must not turn cleanup into a new action.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, edit := range edits {
			if !lease.session.Active() || !blenderBindingMatches(ctx, lease) {
				return
			}
			body, _ := json.Marshal(map[string]string{"editing_session_id": edit[1]})
			_, _ = lease.session.LegacyBlender(ctx, "POST", "/studio/apps/blender/"+edit[0]+"/close", body)
		}
	}()
}

func (b *desktopLegacyBlender) prune() {
	b.mu.Lock()
	var expired []string
	for id, lease := range b.leases {
		if !b.now().Before(lease.expires) || !lease.session.Active() {
			expired = append(expired, id)
		}
	}
	b.mu.Unlock()
	for _, id := range expired {
		b.remove(id)
	}
}

func blenderBindingMatches(ctx context.Context, lease *desktopBlenderLease) bool {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := lease.session.Send(ctx, "POST", "/studio/appcenter/v1/open", lease.request)
	if err != nil || out.Status != 200 || !lease.session.Active() || ctx.Err() != nil {
		return false
	}
	var expected appcenter.OpenRequest
	if json.Unmarshal(lease.request, &expected) != nil {
		return false
	}
	opened, err := decodeLegacyOpened(out.Data, expected)
	return err == nil && opened == lease.opened
}

func (b *desktopLegacyBlender) current(lease *desktopBlenderLease) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.leases[lease.id] == lease && lease.ctx.Err() == nil && b.now().Before(lease.expires) && lease.session.Active()
}

func (b *desktopLegacyBlender) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.URL.RawQuery != "" || !legacyLocalRequest(r, localLegacyOrigin(r), false) || len(r.Header.Values("Range")) != 0 || ((r.Method == "GET" || r.Method == "DELETE") && r.ContentLength != 0) {
		blenderFailure(w, 400, "INVALID_LEGACY_REQUEST")
		return
	}
	b.prune()
	if r.URL.Path == blenderWorkspacePrefix {
		b.open(w, r)
		return
	}
	var lease *desktopBlenderLease
	var tail string
	if strings.HasPrefix(r.URL.Path, "/apps/blender/") {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/apps/blender/"), "/", 3)
		if len(parts) != 3 || !desktopBlenderAlias.MatchString(parts[0]) || !desktopBlenderEdit.MatchString(parts[1]) || r.Method != "GET" {
			blenderFailure(w, 404, "LEGACY_WORKSPACE_GONE")
			return
		}
		b.mu.Lock()
		for _, candidate := range b.leases {
			if edit := candidate.edits[parts[1]]; edit != nil && edit.alias == parts[0] {
				lease = candidate
				break
			}
		}
		b.mu.Unlock()
		tail = "asset"
	} else {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, blenderWorkspacePrefix+"/"), "/", 2)
		id, err := uuid.Parse(parts[0])
		if err != nil || id.Version() != 4 || id.String() != parts[0] {
			blenderFailure(w, 404, "LEGACY_WORKSPACE_GONE")
			return
		}
		b.mu.Lock()
		lease = b.leases[id.String()]
		b.mu.Unlock()
		if len(parts) == 2 {
			tail = parts[1]
		}
	}
	if lease == nil {
		blenderFailure(w, 410, "LEGACY_WORKSPACE_GONE")
		return
	}
	if tail == "" {
		if r.Method != "DELETE" {
			blenderFailure(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		b.remove(lease.id)
		w.WriteHeader(204)
		return
	}
	select {
	case lease.active <- struct{}{}:
		defer func() { <-lease.active }()
	default:
		blenderFailure(w, 429, "LEGACY_WORKSPACE_LIMIT")
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	stop := context.AfterFunc(lease.ctx, cancel)
	defer stop()
	defer cancel()
	if !b.current(lease) || !blenderBindingMatches(ctx, lease) {
		b.remove(lease.id)
		blenderFailure(w, 409, "APPCENTER_BINDING_CHANGED")
		return
	}
	b.mu.Lock()
	if b.leases[lease.id] == lease {
		lease.expires = b.now().Add(30 * time.Minute)
	}
	b.mu.Unlock()
	switch {
	case tail == "signal-shim.js" && r.Method == "GET":
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = w.Write(blenderSignalScript)
	case tail == "requests":
		b.request(w, r.WithContext(ctx), lease)
	case tail == "asset":
		b.asset(w, r.WithContext(ctx), lease)
	case tail == "signals" || strings.HasPrefix(tail, "signals/"):
		b.signal(w, r.WithContext(ctx), lease, strings.TrimPrefix(tail, "signals"))
	default:
		blenderFailure(w, 404, "LEGACY_ROUTE_UNAVAILABLE")
	}
}

func (b *desktopLegacyBlender) open(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		blenderFailure(w, 405, "METHOD_NOT_ALLOWED")
		return
	}
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if len(r.Header.Values("Content-Type")) != 1 || err != nil || kind != "application/json" {
		blenderFailure(w, 400, "INVALID_APPCENTER_REQUEST")
		return
	}
	body, err := strictjson.ReadValue(r.Body, 4<<10)
	var request appcenter.OpenRequest
	if err != nil || appcenter.Decode(bytes.NewReader(body), "open", 4<<10, &request) != nil || request.EntryID != "blender-host-v1" {
		blenderFailure(w, 400, "INVALID_APPCENTER_REQUEST")
		return
	}
	session, err := b.bind()
	if err != nil {
		blenderFailure(w, 401, "UNAUTHENTICATED")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	lease := &desktopBlenderLease{id: uuid.NewString(), session: session, request: body, ctx: ctx, cancel: cancel, expires: b.now().Add(30 * time.Minute), active: make(chan struct{}, 32), edits: map[string]*desktopBlenderEditLease{}}
	checkCtx, checkCancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer checkCancel()
	out, err := session.Send(checkCtx, "POST", "/studio/appcenter/v1/open", body)
	if err != nil || out.Status != 200 {
		cancel()
		blenderFailure(w, 409, "APPCENTER_OPEN_UNAVAILABLE")
		return
	}
	lease.opened, err = decodeLegacyOpened(out.Data, request)
	if err != nil || !session.Active() || checkCtx.Err() != nil {
		cancel()
		blenderFailure(w, 409, "APPCENTER_BINDING_CHANGED")
		return
	}
	b.mu.Lock()
	if len(b.leases) >= 16 {
		b.mu.Unlock()
		cancel()
		blenderFailure(w, 429, "LEGACY_WORKSPACE_LIMIT")
		return
	}
	b.leases[lease.id] = lease
	b.mu.Unlock()
	w.Header().Set("X-Studio-Legacy-Workspace", lease.id)
	w.Header().Set("X-Studio-Legacy-Origin", session.Origin())
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_, _ = w.Write(out.Data)
}

func (b *desktopLegacyBlender) request(w http.ResponseWriter, r *http.Request, lease *desktopBlenderLease) {
	var input struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	}
	if r.Method != "POST" || !blenderDecode(r, 128<<10, &input) || len(input.Body) > 16<<10 || (!strings.HasPrefix(input.Path, "/studio/apps/blender/") && !strings.HasPrefix(input.Path, "/mcp/")) {
		blenderFailure(w, 400, "INVALID_LEGACY_REQUEST")
		return
	}
	var opening, closing string
	parts := strings.Split(strings.TrimPrefix(input.Path, "/studio/apps/blender/"), "/")
	if input.Method == "POST" && len(parts) == 2 && desktopBlenderAlias.MatchString(parts[0]) {
		if parts[1] == "open" {
			opening = parts[0]
		}
		if parts[1] == "close" {
			var body struct {
				ID string `json:"editing_session_id"`
			}
			if strictjson.Decode(strings.NewReader(input.Body), 16<<10, &body) != nil {
				blenderFailure(w, 400, "INVALID_LEGACY_REQUEST")
				return
			}
			b.mu.Lock()
			edit := lease.edits[body.ID]
			valid := edit != nil && edit.alias == parts[0]
			b.mu.Unlock()
			if !valid {
				blenderFailure(w, 403, "LEGACY_EDIT_UNBOUND")
				return
			}
			closing = body.ID
		}
	}
	if opening != "" {
		b.mu.Lock()
		full := len(lease.edits)+lease.opening >= 8
		if !full {
			lease.opening++
		}
		b.mu.Unlock()
		if full {
			blenderFailure(w, 429, "LEGACY_WORKSPACE_LIMIT")
			return
		}
		defer func() { b.mu.Lock(); lease.opening--; b.mu.Unlock() }()
	}
	out, err := lease.session.LegacyBlender(r.Context(), input.Method, input.Path, []byte(input.Body))
	if err != nil {
		blenderFailure(w, 503, "STUDIO_REMOTE_UNAVAILABLE")
		return
	}
	if opening != "" && out.Status == 200 {
		var result struct {
			ID string `json:"editing_session_id"`
		}
		if json.Unmarshal(out.Data, &result) != nil || !desktopBlenderEdit.MatchString(result.ID) {
			blenderFailure(w, 502, "INVALID_LEGACY_RESPONSE")
			return
		}
		b.mu.Lock()
		if b.leases[lease.id] != lease || lease.ctx.Err() != nil || r.Context().Err() != nil || !lease.session.Active() {
			b.mu.Unlock()
			releaseBlenderEdits(lease, [][2]string{{opening, result.ID}})
			blenderFailure(w, 409, "STUDIO_SESSION_CHANGED")
			return
		}
		collision := false
		for _, other := range b.leases {
			if other.edits[result.ID] != nil {
				collision = true
				break
			}
		}
		if !collision {
			lease.edits[result.ID] = &desktopBlenderEditLease{alias: opening, id: result.ID}
		}
		b.mu.Unlock()
		if collision {
			blenderFailure(w, 409, "LEGACY_EDIT_CONFLICT")
			return
		}
	}
	if closing != "" && out.Status >= 200 && out.Status < 300 {
		b.mu.Lock()
		edit := lease.edits[closing]
		delete(lease.edits, closing)
		if edit != nil && edit.signal != nil {
			edit.signal.cancel()
		}
		b.mu.Unlock()
	}
	if !b.current(lease) || r.Context().Err() != nil {
		blenderFailure(w, 409, "STUDIO_SESSION_CHANGED")
		return
	}
	w.Header().Set("Content-Type", out.MIME)
	// Preserve the observation clock and suppress Go's local Date fallback.
	w.Header()["Date"] = nil
	if out.Date != "" {
		w.Header().Set("Date", out.Date)
	}
	w.WriteHeader(out.Status)
	_, _ = w.Write(out.Data)
}

func (b *desktopLegacyBlender) asset(w http.ResponseWriter, r *http.Request, lease *desktopBlenderLease) {
	out, err := lease.session.LegacyBlender(r.Context(), "GET", r.URL.Path, nil)
	if err != nil {
		blenderFailure(w, 503, "STUDIO_REMOTE_UNAVAILABLE")
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/apps/blender/"), "/", 3)
	b.mu.Lock()
	edit := lease.edits[parts[1]]
	bound := edit != nil && edit.alias == parts[0]
	b.mu.Unlock()
	if !bound || !b.current(lease) || r.Context().Err() != nil {
		blenderFailure(w, 409, "STUDIO_SESSION_CHANGED")
		return
	}
	if out.Status == 200 && out.MIME == "text/html" {
		out.Data, err = blenderViewer(out.Data, blenderWorkspacePrefix+"/"+lease.id, parts[0], parts[1])
		if err != nil {
			blenderFailure(w, 502, "INVALID_LEGACY_VIEWER")
			return
		}
	}
	w.Header().Set("Content-Type", out.MIME)
	var viewer []byte
	if out.Status == 200 && out.MIME == "text/html" {
		viewer = out.Data
	}
	w.Header().Set("Content-Security-Policy", blenderViewerCSP(localLegacyOrigin(r), viewer))
	w.WriteHeader(out.Status)
	_, _ = w.Write(out.Data)
}

func (b *desktopLegacyBlender) signal(w http.ResponseWriter, r *http.Request, lease *desktopBlenderLease, tail string) {
	if tail == "" {
		b.openSignal(w, r, lease)
		return
	}
	id := strings.TrimPrefix(tail, "/")
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 4 || parsed.String() != id {
		blenderFailure(w, 404, "LEGACY_SIGNAL_GONE")
		return
	}
	b.mu.Lock()
	var signal *desktopBlenderSignal
	for _, edit := range lease.edits {
		if edit.signal != nil && edit.signal.id == id {
			signal = edit.signal
			break
		}
	}
	b.mu.Unlock()
	if signal == nil {
		blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
		return
	}
	if r.Method == "DELETE" {
		signal.cancel()
		w.WriteHeader(204)
		return
	}
	if signal.ctx.Err() != nil {
		blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
		return
	}
	if r.Method == "GET" {
		select {
		case signal.reading <- struct{}{}:
			defer func() { <-signal.reading }()
		default:
			blenderFailure(w, 409, "LEGACY_SIGNAL_CONCURRENT_READ")
			return
		}
		timer := time.NewTimer(b.pollTimeout)
		defer timer.Stop()
		select {
		case message := <-signal.incoming:
			if !b.current(lease) || signal.ctx.Err() != nil || r.Context().Err() != nil {
				blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
				return
			}
			if blenderJSON(w, 200, map[string]string{"message": message}) != nil {
				signal.cancel()
			}
		case <-timer.C:
			if !b.current(lease) || signal.ctx.Err() != nil {
				blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
				return
			}
			w.WriteHeader(204)
		case <-signal.ctx.Done():
			blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
		case <-r.Context().Done():
			signal.cancel()
			blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
		}
		return
	}
	if r.Method == "POST" {
		var input struct {
			Message string `json:"message"`
		}
		if !blenderDecode(r, 2<<20, &input) || len(input.Message) == 0 || len(input.Message) > 256<<10 {
			signal.cancel()
			blenderFailure(w, 400, "INVALID_LEGACY_SIGNAL")
			return
		}
		select {
		case signal.writing <- struct{}{}:
			defer func() { <-signal.writing }()
		default:
			blenderFailure(w, 409, "LEGACY_SIGNAL_CONCURRENT_WRITE")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if signal.wire.Write(ctx, []byte(input.Message)) != nil || !b.current(lease) || signal.ctx.Err() != nil {
			signal.cancel()
			blenderFailure(w, 410, "LEGACY_SIGNAL_GONE")
			return
		}
		w.WriteHeader(204)
		return
	}
	blenderFailure(w, 405, "METHOD_NOT_ALLOWED")
}

func (b *desktopLegacyBlender) openSignal(w http.ResponseWriter, r *http.Request, lease *desktopBlenderLease) {
	var input struct {
		Alias   string `json:"alias"`
		Session string `json:"editingSession"`
	}
	if r.Method != "POST" || !blenderDecode(r, 1024, &input) {
		blenderFailure(w, 400, "INVALID_LEGACY_SIGNAL")
		return
	}
	b.mu.Lock()
	edit := lease.edits[input.Session]
	valid := edit != nil && edit.alias == input.Alias && !edit.opening && (edit.signal == nil || edit.signal.ctx.Err() != nil)
	if valid {
		edit.opening = true
	}
	b.mu.Unlock()
	if !valid {
		blenderFailure(w, 409, "LEGACY_EDIT_UNBOUND")
		return
	}
	defer func() { b.mu.Lock(); edit.opening = false; b.mu.Unlock() }()
	if edit.signal != nil {
		edit.signal.wire.Close()
	}
	ctx, cancel := context.WithCancel(lease.ctx)
	stop := context.AfterFunc(r.Context(), cancel)
	wire, err := b.dial(ctx, lease.session, input.Alias, input.Session)
	stop()
	if err != nil || ctx.Err() != nil || r.Context().Err() != nil || !b.current(lease) {
		cancel()
		if wire != nil {
			wire.Close()
		}
		blenderFailure(w, 503, "STUDIO_REMOTE_UNAVAILABLE")
		return
	}
	signal := &desktopBlenderSignal{id: uuid.NewString(), wire: wire, ctx: ctx, cancel: cancel, incoming: make(chan string, 32), reading: make(chan struct{}, 1), writing: make(chan struct{}, 1)}
	b.mu.Lock()
	valid = lease.edits[input.Session] == edit && lease.ctx.Err() == nil
	if valid {
		edit.signal = signal
	}
	b.mu.Unlock()
	if !valid {
		cancel()
		wire.Close()
		blenderFailure(w, 410, "LEGACY_EDIT_UNBOUND")
		return
	}
	context.AfterFunc(ctx, wire.Close)
	go func() {
		defer cancel()
		for {
			data, err := wire.Read(ctx)
			if err != nil || len(data) > 256<<10 || !utf8.Valid(data) {
				return
			}
			select {
			case signal.incoming <- string(data):
			case <-ctx.Done():
				return
			default:
				return
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(b.checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if ctx.Err() != nil {
					return
				}
				valid := b.current(lease) && blenderBindingMatches(ctx, lease)
				if ctx.Err() != nil {
					return
				}
				if !valid {
					b.remove(lease.id)
					return
				}
			}
		}
	}()
	if blenderJSON(w, 200, map[string]string{"signalId": signal.id}) != nil {
		cancel()
	}
}
