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
)

func TestAppCenterLegacyOpenRechecksIdentityWorkloadAndBackend(t *testing.T) {
	for _, name := range []string{"valid", "renamed", "unrelated", "workload-race", "backend-race", "identity-race", "service-race", "blocked", "wrong-uid", "missing-backend", "stale-click", "tampered-entry", "adopted-after-click", "installed-valid", "installed-workload-race", "installed-backend-race", "installed-identity-race", "installed-service-race", "installed-revision-race", "installed-version-race", "installed-blocked"} {
		t.Run(name, func(t *testing.T) {
			installed := strings.HasPrefix(name, "installed-")
			scenario := strings.TrimPrefix(name, "installed-")
			raw, err := os.ReadFile("../appcenter/testdata/rules.generated.json")
			if err != nil {
				t.Fatal(err)
			}
			var cases []struct {
				Facts appcenter.Facts `json:"facts"`
			}
			if json.Unmarshal(raw, &cases) != nil {
				t.Fatal("fixture")
			}
			facts := cases[0].Facts
			now := time.Now().UTC()
			facts.GeneratedAt = now
			fact := &facts.Items[0]
			originalInstallation, originalKey := *fact.Installation, fact.CardKey
			fact.Installation = nil
			fact.Origin = "observation"
			fact.CardKey = *fact.Service.ObservationKey
			fact.Service.ObservedAt = &now
			if scenario == "renamed" {
				fact.AppID = "another-app"
			}
			identity := identityContext{UserID: facts.Scope.UserID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID, SessionID: "00000000-0000-7000-8000-000000000009", Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"}, Scopes: []string{"identity:read", "app:read"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
			facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(identity).AuthorizationEpoch
			backend := appcenter.LegacyBackendRevision("image-iframe-v1", "http://sample.invalid:8000")
			binding := appcenter.LegacyBinding{BindingID: "original-deployment", AppID: fact.AppID, OrganizationID: identity.OrganizationID, StationID: identity.StationID, ObservationKey: fact.CardKey, WorkloadUID: *fact.Service.WorkloadUID, AdapterID: "image-iframe-v1", BackendRevision: backend, Route: "/market?image=%2Fdashboard%23tasks"}
			registry := appcenter.LegacyRegistry{SchemaVersion: 1, Revision: "sha256:" + strings.Repeat("a", 64), Records: []appcenter.LegacyBinding{binding}}
			data, _ := json.Marshal(registry)
			path := filepath.Join(t.TempDir(), "legacy.json")
			if os.WriteFile(path, data, 0600) != nil {
				t.Fatal("write")
			}
			adopt := func() {
				fact.Installation = &originalInstallation
				fact.Origin, fact.CardKey = "installation", originalKey
			}
			if installed {
				adopt()
			}
			factCalls, identityCalls := 0, 0
			station := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-session" {
					t.Fatal("unexpected side effect")
				}
				var body []byte
				switch r.URL.Path {
				case "/identity/me":
					identityCalls++
					if scenario == "identity-race" && identityCalls == 2 {
						identity.Roles = nil
					}
					identity.RequestID = r.Header.Get("X-Request-ID")
					body, _ = json.Marshal(identity)
				case "/appcenter/v1/facts":
					factCalls++
					if factCalls == 2 {
						switch scenario {
						case "workload-race":
							facts.Items[0].Service.WorkloadUID = appcenter.Text("00000000-0000-7000-8000-000000000099")
						case "backend-race":
							backend = appcenter.LegacyBackendRevision("image-iframe-v1", "http://replacement.invalid:8000")
						case "service-race":
							facts.Items[0].Service.State = "stopped"
						case "revision-race":
							facts.Items[0].Installation.Revision++
						case "version-race":
							facts.Items[0].Service.WorkloadUID = appcenter.Text("00000000-0000-7000-8000-000000000099")
						}
					}
					facts.RequestID = r.Header.Get("X-Request-ID")
					body, _ = json.Marshal(facts)
				default:
					t.Fatal("legacy open called another endpoint", r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}}
			a := NewAppCenter(station, "").WithLegacy(path, func() map[string]string { return map[string]string{"image-iframe-v1": backend} })
			view, _, out := a.read(context.Background(), "test-session")
			if out.Status != 200 || len(view.Items) != 1 || view.Items[0].PrimaryActionID != "open" || (view.Items[0].Installation != nil) != installed {
				t.Fatal("invalid preview", out.Status, string(out.Data))
			}
			request := appcenter.OpenRequest{SchemaVersion: 1, CardKey: view.Items[0].CardKey, EntryID: "image-iframe-v1", ExpectedViewRevision: view.Revision, ExpectedBindingRevision: view.Items[0].BindingRevision}
			switch scenario {
			case "adopted-after-click":
				adopt()
			case "blocked":
				facts.Items[0].Operation.Blocking = true
			case "wrong-uid":
				facts.Items[0].Service.WorkloadUID = appcenter.Text("00000000-0000-7000-8000-000000000099")
			case "missing-backend":
				backend = ""
			case "stale-click":
				request.ExpectedBindingRevision = "sha256:" + strings.Repeat("f", 64)
			case "tampered-entry":
				request.EntryID = "video-iframe-v1"
			case "unrelated":
				other := facts.Items[0]
				other.CardKey = "obs:" + strings.Repeat("c", 64)
				other.Service.ObservationKey = &other.CardKey
				other.AppID = "other"
				facts.Items = append(facts.Items, other)
			}
			out = a.Open(context.Background(), "test-session", request)
			if scenario != "valid" && scenario != "unrelated" && scenario != "renamed" && scenario != "stale-click" && scenario != "revision-race" {
				if out.Status < 400 {
					t.Fatal("unsafe open allowed")
				}
				return
			}
			var opened appcenter.LegacyOpened
			if out.Status != 200 || json.Unmarshal(out.Data, &opened) != nil || opened.AdapterID != binding.AdapterID || opened.BindingID != binding.BindingID || (opened.BindingRevision != view.Items[0].BindingRevision && scenario != "revision-race") || opened.Route != binding.Route || factCalls != 2 {
				t.Fatal("legacy open mismatch", out.Status, string(out.Data), factCalls)
			}
			if strings.Contains(string(out.Data), "sample.invalid") || strings.Contains(string(out.Data), "test-session") {
				t.Fatal("private backend leaked")
			}
		})
	}
}
