package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
)

type navigationSourceStub struct {
	catalog navigation.Catalog
	err     error
}

func (s *navigationSourceStub) Load(context.Context) (navigation.Catalog, error) {
	return s.catalog, s.err
}

type navigationPreferencesStub struct {
	values   map[string]navigation.Preferences
	revision string
	err      error
	saves    int
	conflict bool
}

func (s *navigationPreferencesStub) Load(_ context.Context, scope navigation.Scope) (navigation.Preferences, string, error) {
	if s.err != nil {
		return navigation.Preferences{}, "", s.err
	}
	prefs, ok := s.values[scope.UserID+scope.OrganizationID+scope.StationID]
	if !ok {
		prefs = navigation.EmptyPreferences(scope)
	}
	prefs.Entries = slices.Clone(prefs.Entries)
	return prefs, s.revision, nil
}
func (s *navigationPreferencesStub) Save(_ context.Context, scope navigation.Scope, prefs navigation.Preferences, revision string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.conflict || revision != s.revision {
		return "", navigation.ErrConflict
	}
	s.saves++
	s.values[scope.UserID+scope.OrganizationID+scope.StationID] = prefs
	s.revision = "2"
	return "2", nil
}
func navigationFixture(t *testing.T) (*Navigation, *dashboardServiceFixture, *navigationPreferencesStub, *navigationSourceStub) {
	t.Helper()
	f := newDashboardServiceFixture(t)
	artifact := "sha256:" + strings.Repeat("a", 64)
	decl := navigation.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: artifact, Entry: navigation.DeclarationEntry{EntryID: "main", Title: "Sample", IconRef: "network", Route: "/", AccessRuleRef: "sample-read"}}
	data, _ := json.Marshal(decl)
	hash := sha256.Sum256(data)
	source := &navigationSourceStub{catalog: navigation.Catalog{Mode: "preinstalled", Releases: map[string]navigation.Binding{"sample@1.0.0": {AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: artifact, DeclarationDigest: "sha256:" + hex.EncodeToString(hash[:]), Declaration: data, Trusted: true, Availability: "unknown"}}, Rules: map[string]navigation.AccessRule{"sample-read": {Roles: []string{"admin"}, Scopes: []string{"app:manage"}}}, Defaults: []navigation.Default{{AppID: "sample", EntryID: "main", Group: "applications", Order: 10, Pinned: true}}}}
	store := &navigationPreferencesStub{values: map[string]navigation.Preferences{}, revision: "0"}
	return &Navigation{station: f.service.station, source: source, preferences: store}, f, store, source
}
func navigationView(t *testing.T, result Result) NavigationResult {
	t.Helper()
	var out NavigationResult
	if result.Status != 200 || json.Unmarshal(result.Data, &out) != nil {
		t.Fatal("navigation failed", result.Status, string(result.Data))
	}
	return out
}

func TestNavigationJoinsRealInstallationAndRetainsUnpinAcrossReinstall(t *testing.T) {
	n, f, store, _ := navigationFixture(t)
	ctx := context.Background()
	view := navigationView(t, n.Get(ctx, "test-session"))
	if len(view.Projection.Entries) != 1 || !view.Projection.Entries[0].Pinned || view.PreferenceRevision != "0" || view.Projection.Entries[0].InstallationID != f.installation.InstallationID {
		t.Fatal("incorrect installed navigation")
	}
	view = navigationView(t, n.Patch(ctx, "test-session", NavigationPreferencePatch{AppID: "sample", EntryID: "main", Pinned: false, Order: 3, PreferenceRevision: "0"}))
	if view.Projection.Entries[0].Pinned || view.Projection.Entries[0].PinSource != "user" || view.PreferenceRevision != "2" || store.saves != 1 {
		t.Fatal("explicit unpin lost")
	}
	f.installation.State = "removed"
	view = navigationView(t, n.Get(ctx, "test-session"))
	if len(view.Projection.Entries) != 0 {
		t.Fatal("removed installation visible")
	}
	f.installation.State = "installed"
	f.installation.InstallationID = uuid.Must(uuid.NewV7()).String()
	view = navigationView(t, n.Get(ctx, "test-session"))
	if len(view.Projection.Entries) != 1 || view.Projection.Entries[0].Pinned || view.Projection.Entries[0].InstallationID != f.installation.InstallationID {
		t.Fatal("reinstall reset preference or reused installation")
	}
	if f.identityCalls != 4 || f.installationCalls != 4 {
		t.Fatal("identity or installation cached")
	}
	for _, private := range []string{"test-session", "svc.cluster.local", "artifactDigest", "accessRuleRef"} {
		if strings.Contains(string(n.Get(ctx, "test-session").Data), private) {
			t.Fatal("private navigation inputs leaked")
		}
	}
}

func TestNavigationRevocationConflictAndUnavailableCannotWrite(t *testing.T) {
	for _, kind := range []string{"permission", "identity", "scope", "duplicate", "empty", "unknown-version", "conflict", "concurrent-conflict", "unavailable", "source", "invalid-order"} {
		t.Run(kind, func(t *testing.T) {
			n, f, store, source := navigationFixture(t)
			patch := NavigationPreferencePatch{AppID: "sample", EntryID: "main", Pinned: false, Order: 3, PreferenceRevision: "0"}
			switch kind {
			case "permission":
				f.identity.Scopes = []string{"identity:read", "app:read"}
			case "identity":
				f.identityStatus = 401
			case "scope":
				f.installation.OrganizationID = uuid.NewString()
			case "duplicate":
				duplicate := f.installation
				duplicate.InstallationID = uuid.Must(uuid.NewV7()).String()
				f.installations = []dashboardInstallation{f.installation, duplicate}
			case "empty":
				f.installations = []dashboardInstallation{}
			case "unknown-version":
				f.installation.AppVersion = "9.0.0"
			case "conflict":
				patch.PreferenceRevision = "7"
			case "concurrent-conflict":
				store.conflict = true
			case "unavailable":
				store.err = navigation.ErrUnavailable
			case "source":
				source.err = errors.New("unavailable")
			case "invalid-order":
				patch.Order = -1
			}
			out := n.Patch(context.Background(), "test-session", patch)
			if out.Status < 400 || store.saves != 0 {
				t.Fatal("unsafe preference write", out.Status)
			}
			if strings.Contains(kind, "conflict") && out.Status != 409 {
				t.Fatal("CAS status changed")
			}
			if kind == "permission" {
				view := navigationView(t, n.Get(context.Background(), "test-session"))
				if len(view.Projection.Entries) != 0 {
					t.Fatal("revoked entry retained")
				}
			}
		})
	}
}

func TestNavigationLegacyModeDoesNotInventInstallationsOrRequirePreferences(t *testing.T) {
	n, f, _, source := navigationFixture(t)
	source.catalog = navigation.Catalog{Mode: "legacy"}
	n.preferences = nil
	f.identity.Roles = []string{"member"}
	f.identity.Scopes = []string{"identity:read"}
	view := navigationView(t, n.Get(context.Background(), "test-session"))
	if view.Mode != "legacy" || len(view.Projection.Entries) != 0 || f.identityCalls != 1 || f.installationCalls != 0 {
		t.Fatal("legacy mode changed existing access or invented installations")
	}
	if out := n.Patch(context.Background(), "test-session", NavigationPreferencePatch{AppID: "sample", EntryID: "main", Order: 0, PreferenceRevision: "0"}); out.Status != 403 {
		t.Fatal("legacy preference write enabled")
	}
}
