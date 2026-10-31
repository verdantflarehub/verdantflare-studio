package application

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

func refreshFixture(t *testing.T, now time.Time) (appcenter.Facts, identityContext) {
	t.Helper()
	raw, err := os.ReadFile("../appcenter/testdata/rules.generated.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Facts appcenter.Facts `json:"facts"`
	}
	if json.Unmarshal(raw, &fixtures) != nil {
		t.Fatal("fixture")
	}
	facts := fixtures[0].Facts
	facts.GeneratedAt = now
	facts.Items[0].Service.ObservedAt = &now
	id := identityContext{UserID: facts.Scope.UserID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID, SessionID: "00000000-0000-7000-8000-000000000009", Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"}, Scopes: []string{"identity:read", "app:read"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
	facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(id).AuthorizationEpoch
	return facts, id
}

type refreshCore struct {
	mu             sync.Mutex
	facts          appcenter.Facts
	identity       identityContext
	identityStatus int
	identityCalls  atomic.Int32
	factCalls      atomic.Int32
	gate           chan struct{}
	now            func() time.Time
}

func (c *refreshCore) station(ttl time.Duration) *Station {
	return &Station{base: "https://core.invalid", identityTTL: ttl, client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		var body []byte
		status := 200
		switch r.URL.Path {
		case "/identity/me":
			c.identityCalls.Add(1)
			if c.gate != nil {
				c.mu.Unlock()
				<-c.gate
				c.mu.Lock()
			}
			if c.identityStatus != 0 {
				status = c.identityStatus
			}
			id := c.identity
			id.RequestID = r.Header.Get("X-Request-ID")
			body, _ = json.Marshal(id)
		case "/appcenter/v1/facts":
			c.factCalls.Add(1)
			facts := c.facts
			facts.RequestID = r.Header.Get("X-Request-ID")
			if c.now != nil {
				at := c.now()
				facts.GeneratedAt = at
				facts.Items = append([]appcenter.Fact(nil), facts.Items...)
				facts.Items[0].Service.ObservedAt = &at
			}
			body, _ = json.Marshal(facts)
		default:
			return nil, http.ErrNotSupported
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}}
}

func TestSessionIdentityCacheSharesForgetsAndRejects(t *testing.T) {
	now := time.Now().UTC()
	_, id := refreshFixture(t, now)
	core := &refreshCore{identity: id, gate: make(chan struct{})}
	s := core.station(identityCacheTTL)
	var wg sync.WaitGroup
	results := make([]identityContext, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got, status := s.sessionIdentity(context.Background(), "token-a", reqID(i))
			if status != 200 {
				t.Error("shared read failed", status)
			}
			results[i] = got
		}(i)
	}
	for core.identityCalls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(core.gate)
	wg.Wait()
	core.gate = nil
	if core.identityCalls.Load() != 1 {
		t.Fatalf("concurrent reads used %d Core calls", core.identityCalls.Load())
	}
	for i, got := range results {
		if got.RequestID != reqID(i) {
			t.Fatal("request ID not rebound per caller", got.RequestID)
		}
	}
	if _, status := s.sessionIdentity(context.Background(), "token-a", reqID(20)); status != 200 || core.identityCalls.Load() != 1 {
		t.Fatal("cached identity not reused")
	}
	s.ForgetIdentity("token-a")
	if _, status := s.sessionIdentity(context.Background(), "token-a", reqID(21)); status != 200 || core.identityCalls.Load() != 2 {
		t.Fatal("forgotten identity reused")
	}
	core.identityStatus = 401
	for range 2 {
		if _, status := s.sessionIdentity(context.Background(), "token-b", reqID(22)); status != 401 {
			t.Fatal("rejected session accepted")
		}
	}
	if core.identityCalls.Load() != 4 {
		t.Fatal("rejection was cached")
	}
	if _, status := s.verifiedIdentity(context.Background(), "token-a", reqID(23)); status != 401 || core.identityCalls.Load() != 5 {
		t.Fatal("proxy path must keep verifying live")
	}
	uncached := core.station(0)
	core.identityStatus = 0
	for range 2 {
		uncached.sessionIdentity(context.Background(), "token-c", reqID(24))
	}
	if core.identityCalls.Load() != 7 {
		t.Fatal("zero TTL must not cache")
	}
}

func TestAppCenterSnapshotCacheServesAndInvalidates(t *testing.T) {
	start := time.Now().UTC()
	facts, id := refreshFixture(t, start)
	clock := start
	core := &refreshCore{facts: facts, identity: id, now: func() time.Time { return clock }}
	a := NewAppCenter(core.station(identityCacheTTL), "").WithSnapshotCache()
	a.now = func() time.Time { return clock }
	watched := map[string]int{}
	a.WatchOrganizations(func(station, organization string) { watched[station+"/"+organization]++ })
	read := func() appcenter.View {
		t.Helper()
		view, _, out := a.read(context.Background(), "test-session")
		if out.Status != 200 {
			t.Fatal("read failed", out.Status, string(out.Data))
		}
		return view
	}
	read()
	read()
	if core.factCalls.Load() != 1 || core.identityCalls.Load() != 1 {
		t.Fatalf("repeat read hit Core: facts=%d identity=%d", core.factCalls.Load(), core.identityCalls.Load())
	}
	if watched[facts.Scope.StationID+"/"+facts.Scope.OrganizationID] != 1 || len(watched) != 1 {
		t.Fatal("cached organization was not watched once", watched)
	}
	// Past Studio's own 15 second window the cached observation still reports
	// Core's state as of the read; Core notifications drive any change.
	clock = start.Add(40 * time.Second)
	view := read()
	if core.factCalls.Load() != 1 || view.Items[0].Service.ReasonCode != nil && *view.Items[0].Service.ReasonCode == "OBSERVATION_EXPIRED" {
		t.Fatal("cached snapshot expired locally", core.factCalls.Load())
	}
	a.InvalidateOrganization(facts.Scope.StationID, facts.Scope.OrganizationID)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); read() }()
	}
	wg.Wait()
	if core.factCalls.Load() != 2 {
		t.Fatalf("invalidated concurrent reads used %d facts calls", core.factCalls.Load()-1)
	}
	a.InvalidateOrganization(facts.Scope.StationID, "00000000-0000-7000-8000-000000000099")
	read()
	if core.factCalls.Load() != 2 {
		t.Fatal("another organization invalidated this scope")
	}
	clock = clock.Add(snapshotMaxAge)
	read()
	if core.factCalls.Load() != 3 {
		t.Fatal("snapshot outlived its maximum age")
	}
	a.InvalidateAll()
	read()
	if core.factCalls.Load() != 4 {
		t.Fatal("reset kept snapshot")
	}
}

func TestAppCenterSnapshotFollowsRegistryChanges(t *testing.T) {
	start := time.Now().UTC()
	facts, id := refreshFixture(t, start)
	core := &refreshCore{facts: facts, identity: id}
	dir := t.TempDir()
	path := dir + "/presentation.json"
	a := NewAppCenter(core.station(identityCacheTTL), path).WithSnapshotCache()
	if _, _, out := a.read(context.Background(), "test-session"); out.Status == 200 {
		t.Fatal("missing registry accepted")
	}
	if core.factCalls.Load() != 1 {
		t.Fatal("unexpected facts calls")
	}
	// Failed loads are never cached.
	a.read(context.Background(), "test-session")
	if core.factCalls.Load() != 2 {
		t.Fatal("failed snapshot cached")
	}
	if sig := fileSignature(path); !strings.HasPrefix(sig, "!") {
		t.Fatal("missing file signature", sig)
	}
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	before := fileSignature(path)
	if err := os.WriteFile(path, []byte("{ }"), 0600); err != nil {
		t.Fatal(err)
	}
	if fileSignature(path) == before {
		t.Fatal("rewritten registry kept its signature")
	}
}

func reqID(i int) string { return fmt.Sprintf("01a19092-0000-7000-8000-%012x", i) }
