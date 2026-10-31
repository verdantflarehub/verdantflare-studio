package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

type appCenterDashboardLookup func(context.Context, dashboard.Actor, string) (dashboard.Binding, error)

func (f appCenterDashboardLookup) Lookup(ctx context.Context, actor dashboard.Actor, id string) (dashboard.Binding, error) {
	return f(ctx, actor, id)
}

func TestAppCenterOpenPinsTargetAndReauthorizes(t *testing.T) {
	for _, scenario := range []string{"valid", "unrelated", "stale-binding", "entry-substitution", "installation-race", "release-race", "scope-race", "blocking", "stopped", "unverified", "invalid-request", "loader-denied", "legacy-present", "legacy-unverified"} {
		t.Run(scenario, func(t *testing.T) {
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
			now := time.Now().UTC()
			facts.GeneratedAt = now
			facts.Items[0].Service.ObservedAt = &now
			identity := identityContext{UserID: facts.Scope.UserID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID, SessionID: "00000000-0000-7000-8000-000000000009", Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"}, Scopes: []string{"identity:read", "app:read"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
			facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(identity).AuthorizationEpoch
			identityCalls := 0
			station := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-session" {
					t.Fatal("unexpected side effect or credential")
				}
				var data []byte
				switch r.URL.Path {
				case "/identity/me":
					identityCalls++
					if scenario == "scope-race" && identityCalls == 3 {
						identity.PolicyVersion++
					}
					identity.RequestID = r.Header.Get("X-Request-ID")
					data, _ = json.Marshal(identity)
				case "/appcenter/v1/facts":
					facts.RequestID = r.Header.Get("X-Request-ID")
					data, _ = json.Marshal(facts)
				default:
					t.Fatal("uncontrolled path")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
			})}}
			digest := "sha256:" + strings.Repeat("a", 64)
			declaration := dashboard.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", Dashboard: &dashboard.Dashboard{EntryRef: "dashboard/index.html", ArtifactDigest: digest, Adapter: "wujie", SDKMajor: 1, MinimumSDKVersion: "1.0.0", RequiredCapabilities: []string{}, OptionalCapabilities: []string{}, AppAPIPolicyRef: "sample-v1"}}
			data, _ := json.Marshal(declaration)
			hash := sha256.Sum256(data)
			declarationDigest := "sha256:" + hex.EncodeToString(hash[:])
			dir := t.TempDir()
			registryPath := filepath.Join(dir, "registry.json")
			if err := os.WriteFile(filepath.Join(dir, hex.EncodeToString(hash[:])+".json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			release := dashboard.RegisteredRelease{AppID: "sample", AppVersion: "1.0.0", DeclarationDigest: declarationDigest, ArtifactDigest: digest, AppAPIPolicyRef: "sample-v1", Grants: []string{}, Rules: []dashboard.Rule{}, Backend: dashboard.Backend{Origin: "http://sample.apps.svc.cluster.local", TokenEnv: "STUDIO_APP_SAMPLE_TOKEN", DeclarationPath: "/declaration", IndexPath: "/index", AssetPath: "/assets/", AssetPrefix: "dashboard/"}}
			data, _ = json.Marshal(dashboard.Registry{SchemaVersion: 1, Releases: []dashboard.RegisteredRelease{release}})
			if err := os.WriteFile(registryPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			d := NewDashboards(station, registryPath)
			defer d.Shutdown()
			lookupCalls := 0
			d.resolver = dashboard.New(appCenterDashboardLookup(func(_ context.Context, actor dashboard.Actor, id string) (dashboard.Binding, error) {
				lookupCalls++
				i := facts.Items[0].Installation
				binding := dashboard.Binding{InstallationID: id, AppID: "sample", AppVersion: "1.0.0", StationID: actor.StationID, OrganizationID: actor.OrganizationID, ArtifactDigest: digest, Revision: digest, Declaration: declaration, Trusted: true, Grants: []string{}, Rules: []dashboard.Rule{}, InstallationRevision: i.Revision, ReleaseRevision: appcenter.Digest(release)}
				if scenario == "installation-race" {
					binding.InstallationRevision++
				}
				if scenario == "release-race" {
					binding.ReleaseRevision = "sha256:" + strings.Repeat("b", 64)
				}
				if scenario == "loader-denied" {
					return dashboard.Binding{}, dashboard.ErrDenied
				}
				return binding, nil
			}))
			a := NewAppCenter(station, "").WithDashboards(d, dir)
			if strings.HasPrefix(scenario, "legacy-") {
				fact := facts.Items[0]
				backend := appcenter.LegacyBackendRevision("image-iframe-v1", "http://sample.invalid:8000")
				binding := appcenter.LegacyBinding{BindingID: "original", AppID: fact.AppID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID, ObservationKey: *fact.Service.ObservationKey, WorkloadUID: *fact.Service.WorkloadUID, AdapterID: "image-iframe-v1", BackendRevision: backend, Route: "/market?image=%2Fdashboard%23tasks"}
				data, _ := json.Marshal(appcenter.LegacyRegistry{SchemaVersion: 1, Revision: digest, Records: []appcenter.LegacyBinding{binding}})
				legacyPath := filepath.Join(dir, "legacy.json")
				if err := os.WriteFile(legacyPath, data, 0600); err != nil {
					t.Fatal(err)
				}
				a.WithLegacy(legacyPath, func() map[string]string { return map[string]string{"image-iframe-v1": backend} })
			}
			view, _, out := a.read(context.Background(), "test-session")
			if out.Status != 200 {
				t.Fatal("initial view", out.Status, string(out.Data))
			}
			if view.Items[0].PrimaryActionID != "open" || lookupCalls != 0 {
				t.Fatal("metadata did not enable offline open")
			}
			request := appcenter.OpenRequest{SchemaVersion: 1, CardKey: view.Items[0].CardKey, EntryID: "dashboard", ExpectedViewRevision: view.Revision, ExpectedBindingRevision: view.Items[0].BindingRevision}
			switch scenario {
			case "unrelated":
				other := facts.Items[0]
				other.AppID = "unrelated"
				other.Installation = nil
				other.Origin = "catalog"
				other.CardKey = "catalog:unrelated"
				facts.Items = append(facts.Items, other)
			case "stale-binding":
				facts.Items[0].Installation.Revision++
			case "entry-substitution":
				request.EntryID = "legacy"
			case "blocking":
				facts.Items[0].Operation.Blocking = true
			case "stopped":
				facts.Items[0].Service.State = "stopped"
			case "unverified", "legacy-unverified":
				a.declarationsDir = ""
			case "invalid-request":
				request.SchemaVersion = 2
			}
			out = a.Open(context.Background(), "test-session", request)
			want, lookups := 409, 0
			switch scenario {
			// A stopped service or pending operation is reported inside the App,
			// not by refusing the static Open.
			case "valid", "unrelated", "legacy-present", "stale-binding", "installation-race", "release-race", "blocking", "stopped":
				want, lookups = 200, 1
			case "invalid-request":
				want = 400
			case "loader-denied":
				want, lookups = 403, 1
			}
			if out.Status != want || lookupCalls != lookups {
				t.Fatalf("status %d want %d lookup %d want %d: %s", out.Status, want, lookupCalls, lookups, out.Data)
			}
			if want == 200 {
				var opened struct {
					SchemaVersion int             `json:"schemaVersion"`
					Transport     string          `json:"transport"`
					Opened        DashboardOpened `json:"opened"`
				}
				if json.Unmarshal(out.Data, &opened) != nil || opened.Transport != "dashboard" || opened.Opened.Launch.InstallationID != facts.Items[0].Installation.InstallationID || opened.Opened.Context.ContextRevision != facts.Scope.AuthorizationEpoch {
					t.Fatal("wrong launch")
				}
			}
		})
	}
}
