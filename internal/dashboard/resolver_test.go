package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type sourceFunc func(context.Context, Actor, string) (Binding, error)

func (f sourceFunc) Lookup(ctx context.Context, actor Actor, id string) (Binding, error) {
	return f(ctx, actor, id)
}

func fixture() (Actor, Binding) {
	actor := Actor{"user-a", "org-a", "station-a", "session-a:policy-1"}
	artifact := "sha256:" + strings.Repeat("a", 64)
	return actor, Binding{
		InstallationID: "install-a", AppID: "sample", AppVersion: "1.2.3",
		StationID: actor.StationID, OrganizationID: actor.OrganizationID,
		ArtifactDigest: artifact, Revision: "revision-1", Trusted: true,
		Grants: []string{"app.request", "instances.open", "files.select"},
		Rules:  []Rule{{Method: "GET", Path: "/instances", UpstreamPath: "/internal/instances"}, {Method: "POST", Path: "/instances/:instanceId/start", UpstreamPath: "/internal/instances/:instanceId/start", MaxBody: 256}},
		Declaration: Declaration{1, "sample", "1.2.3", &Dashboard{
			EntryRef: "dashboard/console.html", ArtifactDigest: artifact,
			Adapter: "wujie", SDKMajor: 1, MinimumSDKVersion: "1.0.0",
			RequiredCapabilities: []string{"app.request", "instances.open"},
			OptionalCapabilities: []string{"navigation.request"},
			AppAPIPolicyRef:      "sample-management-v1", KeepAlive: true,
		}},
	}
}

func copyBinding(binding Binding) Binding {
	data, _ := json.Marshal(binding)
	var result Binding
	if err := json.Unmarshal(data, &result); err != nil {
		panic(err)
	}
	return result
}

func resolverFor(binding *Binding) *Resolver {
	return New(sourceFunc(func(context.Context, Actor, string) (Binding, error) {
		return copyBinding(*binding), nil
	}))
}

func openFixture(t *testing.T, resolver *Resolver, actor Actor) Launch {
	t.Helper()
	launch, err := resolver.Open(context.Background(), actor, "install-a")
	if err != nil {
		t.Fatal(err)
	}
	return launch
}

func TestLaunchUsesInstalledEntryAndNegotiatedGrants(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	now := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return now }
	launch := openFixture(t, r, actor)
	if launch.EntryURL != "/studio/dashboards/"+launch.DashboardSessionID+"/assets/dashboard/console.html" {
		t.Fatalf("entry does not preserve package path: %s", launch.EntryURL)
	}
	if len(launch.GrantedCapabilities) != 2 || launch.ContextRevision != actor.AuthorizationEpoch || !launch.ExpiresAt.Equal(now.Add(15*time.Minute)) {
		t.Fatalf("invalid launch: %+v", launch)
	}
	// Mutating the caller's projection cannot grant new server-side capabilities.
	launch.GrantedCapabilities[0] = "files.select"
	if _, err := r.AuthorizeRequest(context.Background(), actor, launch.DashboardSessionID, "GET", "/instances", 0); err != nil {
		t.Fatal(err)
	}
}

func TestFileSDKVersionAndCapabilitiesMustBeNegotiated(t *testing.T) {
	actor, binding := fixture()
	binding.Declaration.Dashboard.MinimumSDKVersion = "1.2.0"
	binding.Declaration.Dashboard.RequiredCapabilities = []string{"files.select", "files.read"}
	r := resolverFor(&binding)
	if _, err := r.Open(context.Background(), actor, "install-a"); !errors.Is(err, ErrDenied) {
		t.Fatalf("missing read grant accepted: %v", err)
	}
	binding.Grants = append(binding.Grants, "files.read")
	launch := openFixture(t, r, actor)
	if launch.SDKVersion != "1.2.0" || len(launch.GrantedCapabilities) != 2 {
		t.Fatal("file capabilities were not negotiated")
	}
	binding.Declaration.Dashboard.MinimumSDKVersion = "1.3.0"
	if _, err := r.Open(context.Background(), actor, "install-a"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("future SDK was accepted: %v", err)
	}
}

func TestInspectRevalidatesWithoutRenewingOrAllocating(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	now := time.Now()
	r.now = func() time.Time { return now }
	first := openFixture(t, r, actor)
	now = now.Add(time.Minute)
	inspected, err := r.Inspect(context.Background(), actor, first.DashboardSessionID)
	if err != nil || inspected.DashboardSessionID != first.DashboardSessionID || !inspected.ExpiresAt.Equal(first.ExpiresAt) || len(r.sessions) != 1 {
		t.Fatal("inspection renewed session", err)
	}
	inspected.GrantedCapabilities[0] = "ungranted"
	if again, err := r.Inspect(context.Background(), actor, first.DashboardSessionID); err != nil || again.GrantedCapabilities[0] == "ungranted" {
		t.Fatal("caller changed stored grants", err)
	}
	binding.Revision = "changed"
	if _, err := r.Inspect(context.Background(), actor, first.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatal("stale binding accepted", err)
	}
	if len(r.sessions) != 0 {
		t.Fatal("invalid session not removed")
	}
}

func TestBindingTrustScopeAndDeclarationFailures(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Binding)
		want   error
	}{
		{"untrusted", func(b *Binding) { b.Trusted = false }, ErrDenied},
		{"other installation", func(b *Binding) { b.InstallationID = "install-b" }, ErrDenied},
		{"other station", func(b *Binding) { b.StationID = "station-b" }, ErrDenied},
		{"other organization", func(b *Binding) { b.OrganizationID = "org-b" }, ErrDenied},
		{"wrong app", func(b *Binding) { b.Declaration.AppID = "other" }, ErrInvalid},
		{"wrong version", func(b *Binding) { b.Declaration.AppVersion = "1.2.4" }, ErrInvalid},
		{"wrong digest", func(b *Binding) { b.ArtifactDigest = "sha256:" + strings.Repeat("b", 64) }, ErrInvalid},
		{"no dashboard", func(b *Binding) { b.Declaration.Dashboard = nil }, ErrUnsupported},
		{"unsupported adapter", func(b *Binding) { b.Declaration.Dashboard.Adapter = "isolated-iframe" }, ErrUnsupported},
		{"unsupported sdk", func(b *Binding) { b.Declaration.Dashboard.MinimumSDKVersion = "1.3.0" }, ErrUnsupported},
		{"unbounded timeout", func(b *Binding) { b.Rules[0].TimeoutSeconds = 61 }, ErrInvalid},
		{"required capability missing", func(b *Binding) { b.Grants = []string{"app.request"} }, ErrDenied},
		{"duplicate grant", func(b *Binding) { b.Grants = append(b.Grants, "app.request") }, ErrInvalid},
		{"overlapping requests", func(b *Binding) { b.Declaration.Dashboard.OptionalCapabilities = []string{"app.request"} }, ErrInvalid},
		{"entry traversal", func(b *Binding) { b.Declaration.Dashboard.EntryRef = "dashboard/../index.html" }, ErrInvalid},
		{"entry protocol", func(b *Binding) { b.Declaration.Dashboard.EntryRef = "https://other.invalid/index.html" }, ErrInvalid},
		{"entry empty segment", func(b *Binding) { b.Declaration.Dashboard.EntryRef = "dashboard//index.html" }, ErrInvalid},
		{"policy empty parameter", func(b *Binding) { b.Rules[1].Path = "/instances/:/start" }, ErrInvalid},
		{"read body", func(b *Binding) { b.Rules[0].MaxBody = 1 }, ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actor, binding := fixture()
			tc.change(&binding)
			if _, err := resolverFor(&binding).Open(context.Background(), actor, "install-a"); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	actor, _ := fixture()
	if _, err := New(nil).Open(context.Background(), actor, "install-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestEveryRequestRevalidatesIdentityAndBinding(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	launch := openFixture(t, r, actor)
	for _, changed := range []Actor{
		{"user-b", actor.OrganizationID, actor.StationID, actor.AuthorizationEpoch},
		{actor.UserID, "org-b", actor.StationID, actor.AuthorizationEpoch},
		{actor.UserID, actor.OrganizationID, "station-b", actor.AuthorizationEpoch},
		{actor.UserID, actor.OrganizationID, actor.StationID, "session-b:policy-1"},
	} {
		if _, err := r.Check(context.Background(), changed, launch.DashboardSessionID); !errors.Is(err, ErrDenied) {
			t.Fatalf("identity change allowed: %v", err)
		}
	}
	// A policy edit must invalidate a session even if a provider forgets to bump revision.
	binding.Rules[1].MaxBody = 512
	if _, err := r.Check(context.Background(), actor, launch.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatalf("policy change: %v", err)
	}
	binding.Rules[1].MaxBody = 256
	if _, err := r.Check(context.Background(), actor, launch.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatalf("restoring policy revived the old session: %v", err)
	}
	launch = openFixture(t, r, actor)
	binding.Trusted = false
	if _, err := r.Check(context.Background(), actor, launch.DashboardSessionID); !errors.Is(err, ErrDenied) {
		t.Fatalf("revocation: %v", err)
	}
	binding.Trusted = true
	if _, err := r.Check(context.Background(), actor, launch.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatalf("restoring trust revived the old session: %v", err)
	}
}

func TestRequestAllowlistDoesNotNormalizeOrBroadenPaths(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	launch := openFixture(t, r, actor)
	for _, path := range []string{"/instances/one/start", "/instances/two/start"} {
		if _, err := r.AuthorizeRequest(context.Background(), actor, launch.DashboardSessionID, "POST", path, 256); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		method, path string
		size         int
	}{
		{"POST", "/instances/one/start", 257}, {"POST", "/instances/one/stop", 0},
		{"GET", "/instances", 1}, {"GET", "/instances", -1}, {"GET", "/instances?upstream=other", 0},
		{"GET", "//instances", 0}, {"GET", "/instances/", 0}, {"GET", "/instances/../instances", 0},
		{"POST", "/instances//start", 0}, {"POST", "/instances/%2e%2e/start", 0},
		{"POST", "/instances/./start", 0}, {"POST", "/instances/one/start\x7f", 0},
		{"POST", "/instances/one/start\v", 0}, {"POST", "/instances/one\\start", 0},
		{"post", "/instances/one/start", 0}, {"GET", "https://other.invalid/instances", 0},
	}
	for _, tc := range cases {
		if _, err := r.AuthorizeRequest(context.Background(), actor, launch.DashboardSessionID, tc.method, tc.path, tc.size); !errors.Is(err, ErrDenied) {
			t.Errorf("%q %q (%d): %v", tc.method, tc.path, tc.size, err)
		}
	}
}

func TestCloseDuringLookupAndExpiryRevokePendingRequest(t *testing.T) {
	for _, mode := range []string{"close", "expire", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			actor, binding := fixture()
			now := time.Now()
			var duringLookup func()
			r := New(sourceFunc(func(context.Context, Actor, string) (Binding, error) {
				if duringLookup != nil {
					duringLookup()
				}
				return copyBinding(binding), nil
			}))
			r.now = func() time.Time { return now }
			launch := openFixture(t, r, actor)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			want := ErrExpired
			switch mode {
			case "close":
				duringLookup = func() {
					if err := r.Close(actor, launch.DashboardSessionID); err != nil {
						t.Fatal(err)
					}
				}
			case "expire":
				duringLookup = func() { now = now.Add(15 * time.Minute) }
			case "cancel":
				duringLookup = cancel
				want = context.Canceled
			}
			if _, err := r.Check(ctx, actor, launch.DashboardSessionID); !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
		})
	}
}

func TestCloseOnlyRemovesTheOwnedDashboardSession(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	first, second := openFixture(t, r, actor), openFixture(t, r, actor)
	other := actor
	other.UserID = "user-b"
	if err := r.Close(other, first.DashboardSessionID); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if err := r.Close(actor, first.DashboardSessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Check(context.Background(), actor, first.DashboardSessionID); !errors.Is(err, ErrExpired) {
		t.Fatal(err)
	}
	if _, err := r.Check(context.Background(), actor, second.DashboardSessionID); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCapacityReclaimsExpiredEntries(t *testing.T) {
	actor, binding := fixture()
	r := resolverFor(&binding)
	now := time.Now()
	r.now = func() time.Time { return now }
	for range 1024 {
		openFixture(t, r, actor)
	}
	if _, err := r.Open(context.Background(), actor, "install-a"); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	now = now.Add(15 * time.Minute)
	openFixture(t, r, actor)
	if len(r.sessions) != 1 {
		t.Fatalf("expired entries retained: %d", len(r.sessions))
	}
}

func TestDecodeDeclarationRejectsUnknownMissingAndDuplicateFields(t *testing.T) {
	_, binding := fixture()
	data, _ := json.Marshal(binding.Declaration)
	valid := string(data)
	if _, err := DecodeDeclaration(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"unknown":true`, 1),
		strings.Replace(valid, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(valid, `"keepAlive":true`, `"keepAlive":null`, 1),
		strings.Replace(valid, `,"keepAlive":true`, ``, 1),
		strings.Replace(valid, `"appId"`, `"AppId"`, 1),
		valid + `{}`, strings.Repeat(" ", 64<<10) + valid,
	} {
		if _, err := DecodeDeclaration(strings.NewReader(invalid)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid wire shape accepted: %v", err)
		}
	}
}
