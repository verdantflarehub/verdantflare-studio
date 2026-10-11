package application

import (
	"context"
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
	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
)

func TestNavigationConsumesAppCenterAndKeepsPreferences(t *testing.T) {
	for _, scenario := range []string{"ready", "stopped", "blocking", "unknown", "unverified", "removed", "artifact-race", "identity-race", "failed-facts", "unpin"} {
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
			facts.GeneratedAt, facts.Items[0].Service.ObservedAt = now, &now
			identity := identityContext{UserID: facts.Scope.UserID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID, SessionID: "00000000-0000-7000-8000-000000000009", Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"}, Scopes: []string{"identity:read", "app:read"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
			facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(identity).AuthorizationEpoch
			calls, factCalls := 0, 0
			station := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-session" {
					t.Fatal("unexpected effect")
				}
				var body []byte
				status := 200
				switch r.URL.Path {
				case "/identity/me":
					calls++
					if scenario == "identity-race" && calls == 2 {
						identity.PolicyVersion++
						facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(identity).AuthorizationEpoch
					}
					identity.RequestID = r.Header.Get("X-Request-ID")
					body, _ = json.Marshal(identity)
				case "/appcenter/v1/facts":
					factCalls++
					facts.RequestID = r.Header.Get("X-Request-ID")
					if scenario == "failed-facts" {
						status = 503
					}
					body, _ = json.Marshal(facts)
				default:
					t.Fatal("second installation source or unexpected upstream", r.URL.Path)
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}}
			digest := "sha256:" + strings.Repeat("a", 64)
			decl := dashboard.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", Dashboard: &dashboard.Dashboard{EntryRef: "dashboard/index.html", ArtifactDigest: digest, Adapter: "wujie", SDKMajor: 1, MinimumSDKVersion: "1.0.0", RequiredCapabilities: []string{}, OptionalCapabilities: []string{}, AppAPIPolicyRef: "sample-v1"}}
			declRaw, _ := json.Marshal(decl)
			declDigest := appcenter.Digest(decl)
			dir := t.TempDir()
			if scenario != "unverified" {
				if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(declDigest, "sha256:")+".json"), declRaw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			release := dashboard.RegisteredRelease{AppID: "sample", AppVersion: "1.0.0", DeclarationDigest: declDigest, ArtifactDigest: digest, AppAPIPolicyRef: "sample-v1", Grants: []string{}, Rules: []dashboard.Rule{}, Backend: dashboard.Backend{Origin: "http://sample.apps.svc.cluster.local", TokenEnv: "STUDIO_APP_SAMPLE_TOKEN", DeclarationPath: "/declaration", IndexPath: "/index", AssetPath: "/assets/", AssetPrefix: "dashboard/"}}
			registry, _ := json.Marshal(dashboard.Registry{SchemaVersion: 1, Releases: []dashboard.RegisteredRelease{release}})
			path := filepath.Join(dir, "registry.json")
			if err := os.WriteFile(path, registry, 0600); err != nil {
				t.Fatal(err)
			}
			d := NewDashboards(station, path)
			defer d.Shutdown()
			a := NewAppCenter(station, "").WithDashboards(d, dir)
			navDecl := navigation.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: digest, Entry: navigation.DeclarationEntry{EntryID: "main", Title: "Sample", IconRef: "app", Route: "/", AccessRuleRef: "sample-read"}}
			navRaw, _ := json.Marshal(navDecl)
			binding := navigation.Binding{AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: digest, DeclarationDigest: appcenter.Digest(navDecl), Declaration: navRaw, Trusted: true, Availability: "unknown"}
			if scenario == "artifact-race" {
				binding.ArtifactDigest = "sha256:" + strings.Repeat("b", 64)
			}
			source := &navigationSourceStub{catalog: navigation.Catalog{Mode: "preinstalled", Releases: map[string]navigation.Binding{"sample@1.0.0": binding}, Rules: map[string]navigation.AccessRule{"sample-read": {Roles: []string{"admin"}, Scopes: []string{"app:read"}}}, Defaults: []navigation.Default{{AppID: "sample", EntryID: "main", Group: "applications", Pinned: true, Order: 2}}}}
			store := &navigationPreferencesStub{values: map[string]navigation.Preferences{}, revision: "0"}
			n := (&Navigation{station: station, source: source, preferences: store}).WithAppCenter(a)
			switch scenario {
			case "stopped":
				facts.Items[0].Service.State = "stopped"
			case "blocking":
				facts.Items[0].Operation.Blocking = true
			case "unknown":
				facts.Items[0].Service.Quality = "unknown"
			case "removed":
				facts.Items[0].Installation.State = "removed"
			}
			out := n.Get(context.Background(), "test-session")
			if scenario == "artifact-race" || scenario == "identity-race" || scenario == "failed-facts" {
				if out.Status < 400 || store.saves != 0 {
					t.Fatal("mixed or failed snapshot accepted")
				}
				return
			}
			v := navigationView(t, out)
			if factCalls != 1 {
				t.Fatal("not one common fact snapshot")
			}
			if scenario == "removed" {
				if len(v.Projection.Entries) != 0 {
					t.Fatal("removed installation retained")
				}
				return
			}
			if len(v.Projection.Entries) != 1 {
				t.Fatal("unavailable menu disappeared")
			}
			entry := v.Projection.Entries[0]
			want := "unknown"
			if scenario == "ready" || scenario == "unpin" {
				want = "ready"
			}
			if scenario == "stopped" {
				want = "offline"
			}
			if entry.Availability != want || !entry.Pinned || entry.InstallationID != facts.Items[0].Installation.InstallationID {
				t.Fatal("menu and card differ", entry)
			}
			if scenario == "unpin" {
				v = navigationView(t, n.Patch(context.Background(), "test-session", NavigationPreferencePatch{AppID: "sample", EntryID: "main", Pinned: false, Order: 9, PreferenceRevision: "0"}))
				if v.Projection.Entries[0].Pinned || store.saves != 1 || v.Projection.Entries[0].Order != 9 {
					t.Fatal("preference lost")
				}
				facts.Items[0].Service.State = "stopped"
				v = navigationView(t, n.Get(context.Background(), "test-session"))
				if v.Projection.Entries[0].Pinned || v.Projection.Entries[0].Availability != "offline" {
					t.Fatal("offline reset preference")
				}
			}
		})
	}
}
