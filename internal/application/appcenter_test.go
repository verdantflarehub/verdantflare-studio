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
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
)

type appCenterTransport func(*http.Request) (*http.Response, error)

func (f appCenterTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAppCenterAuthenticatedProjection(t *testing.T) {
	for _, scenario := range []string{"ready", "empty", "denied", "unavailable", "old-core", "wrong-scope", "wrong-request", "duplicate-field", "expired", "invalid-state", "preinstalled", "preinstalled-other-scope", "preinstalled-other-app", "preinstalled-empty-facts", "preinstalled-invalid", "preinstalled-unavailable"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now().UTC()
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
			calls := []string{}
			s := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
				calls = append(calls, r.URL.Path)
				if r.Header.Get("Authorization") != "Bearer test-session" || r.Method != "GET" {
					t.Fatal("wrong authentication or method")
				}
				var body []byte
				status := 200
				switch r.URL.Path {
				case "/identity/me":
					if scenario == "denied" {
						id.Roles = []string{"member"}
					}
					id.RequestID = r.Header.Get("X-Request-ID")
					body, _ = json.Marshal(id)
				case "/appcenter/v1/facts":
					facts.RequestID = r.Header.Get("X-Request-ID")
					switch scenario {
					case "empty", "preinstalled-empty-facts":
						facts.Items = []appcenter.Fact{}
					case "unavailable":
						status = 503
					case "old-core":
						status = 404
					case "wrong-scope":
						facts.Scope.OrganizationID = "00000000-0000-7000-8000-000000000099"
					case "wrong-request":
						facts.RequestID = "other-request"
					case "expired":
						facts.GeneratedAt = now.Add(-16 * time.Second)
					case "invalid-state":
						facts.Items[0].Service.State = "invented"
					}
					body, _ = json.Marshal(facts)
					if scenario == "duplicate-field" {
						body = []byte(strings.Replace(string(body), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
					}
				default:
					t.Fatal("unexpected upstream path")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
			})}}
			a := NewAppCenter(s, "")
			if strings.HasPrefix(scenario, "preinstalled") {
				path := filepath.Join(t.TempDir(), "preinstalled.json")
				config := preinstalled.Config{SchemaVersion: 1, ConfigID: "system", Revision: "1.0.0", OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID,
					Apps: []preinstalled.Release{{AppID: facts.Items[0].AppID, AppVersion: "9.0.0", ReleaseRef: "sample-9", PackageDigest: "sha256:" + strings.Repeat("a", 64), ChartDigest: "sha256:" + strings.Repeat("b", 64), Dependencies: []string{}}}}
				if scenario == "preinstalled-other-scope" {
					config.OrganizationID = "00000000-0000-7000-8000-000000000099"
				}
				if scenario == "preinstalled-other-app" {
					config.Apps[0].AppID = "other"
				}
				if scenario == "preinstalled-invalid" {
					config.SchemaVersion = 9
				}
				data, _ := json.Marshal(config)
				if scenario != "preinstalled-unavailable" {
					if err := os.WriteFile(path, data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				a.WithPreinstalled(path)
			}
			a.now = func() time.Time { return now }
			out := a.Get(context.Background(), "test-session")
			want := 502
			switch scenario {
			case "ready", "empty", "preinstalled", "preinstalled-other-scope", "preinstalled-other-app", "preinstalled-empty-facts":
				want = 200
			case "denied":
				want = 403
			case "unavailable", "expired", "preinstalled-unavailable":
				want = 503
			}
			if out.Status != want {
				t.Fatalf("status=%d want=%d data=%s", out.Status, want, out.Data)
			}
			if scenario == "denied" {
				if len(calls) != 1 {
					t.Fatal("permission failure read facts")
				}
				return
			}
			if len(calls) != 2 {
				t.Fatal("unexpected calls", calls)
			}
			if want == 200 {
				var view appcenter.View
				if json.Unmarshal(out.Data, &view) != nil {
					t.Fatal("view")
				}
				if !view.Partial {
					t.Fatal("missing entry provider hidden")
				}
				for _, card := range view.Items {
					if card.Preinstalled != (scenario == "preinstalled") {
						t.Fatal("wrong preinstalled policy or scope")
					}
					if strings.HasPrefix(scenario, "preinstalled") && (card.InstalledVersion == nil || *card.InstalledVersion != "1.0.0") {
						t.Fatal("policy replaced installation version")
					}
					for _, action := range card.Actions {
						if action.Kind == "open" && action.Enabled {
							t.Fatal("entry fabricated")
						}
					}
				}
				if (scenario == "empty" || scenario == "preinstalled-empty-facts") && len(view.Items) != 0 {
					t.Fatal("successful empty list fabricated cards")
				}
			}
		})
	}
}
