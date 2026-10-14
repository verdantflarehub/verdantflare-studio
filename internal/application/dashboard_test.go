package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

type dashboardServiceFixture struct {
	service                            *Dashboards
	identity                           identityContext
	installation                       dashboardInstallation
	installations                      []dashboardInstallation
	identityCalls, installationCalls   int
	identityStatus, installationStatus int
	mutatePayload                      func(string) string
}

func newDashboardServiceFixture(t *testing.T) *dashboardServiceFixture {
	t.Helper()
	i := dashboardIdentityFixture()
	i.Roles = []string{"admin"}
	i.Scopes = append(i.Scopes, "app:manage")
	f := &dashboardServiceFixture{identity: i, identityStatus: 200, installationStatus: 200,
		installation: dashboardInstallation{InstallationID: uuid.Must(uuid.NewV7()).String(), AppID: "sample", AppVersion: "1.0.0",
			StationID: i.StationID, OrganizationID: i.OrganizationID, State: "installed", Revision: 3, LastOperationID: uuid.Must(uuid.NewV7()).String(),
			CreatedAt: time.Now().Add(-time.Hour).UTC(), UpdatedAt: time.Now().Add(-time.Minute).UTC()}}
	station := &Station{base: "https://station.invalid", client: &http.Client{Transport: dashboardIdentityTransport(func(req *http.Request) (*http.Response, error) {
		if req.Method != "GET" || req.Header.Get("Authorization") != "Bearer test-session" || req.Header.Get("X-Request-ID") == "" || req.Header.Get("Cookie") != "" {
			t.Fatal("unexpected authenticated request")
		}
		var payload string
		status := 200
		switch req.URL.Path {
		case "/identity/me":
			f.identityCalls++
			payload = identityJSON(f.identity)
			status = f.identityStatus
		case "/app-installations":
			f.installationCalls++
			items := f.installations
			if items == nil {
				items = []dashboardInstallation{f.installation}
			}
			data, err := json.Marshal(map[string]any{"request_id": req.Header.Get("X-Request-ID"), "station_id": f.identity.StationID, "items": items})
			if err != nil {
				t.Fatal(err)
			}
			payload = string(data)
			status = f.installationStatus
			if f.mutatePayload != nil {
				payload = f.mutatePayload(payload)
			}
		case "/app-installations/" + f.installation.InstallationID:
			f.installationCalls++
			data, err := json.Marshal(map[string]any{"request_id": req.Header.Get("X-Request-ID"), "installation": f.installation})
			if err != nil {
				t.Fatal(err)
			}
			payload = string(data)
			status = f.installationStatus
			if f.mutatePayload != nil {
				payload = f.mutatePayload(payload)
			}
		default:
			t.Fatal("uncontrolled Core path")
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}, nil
	})}}
	f.service = NewDashboards(station, "")
	t.Cleanup(f.service.Shutdown)
	return f
}

func TestDashboardInstallationReaderUsesVerifiedRequestCredentials(t *testing.T) {
	f := newDashboardServiceFixture(t)
	ctx, actor, result := f.service.begin(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatal(result.Status)
	}
	i, err := f.service.readInstallation(ctx, actor, f.installation.InstallationID)
	if err != nil || i.AppID != "sample" || i.Revision != 3 || !i.CanManage {
		t.Fatal("installation not bound", err)
	}
	if f.identityCalls != 1 || f.installationCalls != 1 {
		t.Fatal("missing identity or installation request")
	}
	f.identity.Scopes = []string{"identity:read", "app:read"}
	ctx, actor, result = f.service.begin(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatal(result.Status)
	}
	i, err = f.service.readInstallation(ctx, actor, f.installation.InstallationID)
	if err != nil || i.CanManage {
		t.Fatal("old write privilege retained", err)
	}
	if f.identityCalls != 2 || f.installationCalls != 2 {
		t.Fatal("identity cached")
	}
}

func dashboardListFixture(t *testing.T) *dashboardServiceFixture {
	t.Helper()
	f := newDashboardServiceFixture(t)
	registration := dashboard.Registry{SchemaVersion: 1, Releases: []dashboard.RegisteredRelease{{
		AppID: f.installation.AppID, AppVersion: f.installation.AppVersion, DeclarationDigest: "sha256:" + strings.Repeat("a", 64), ArtifactDigest: "sha256:" + strings.Repeat("b", 64),
		AppAPIPolicyRef: "sample-v1", Grants: []string{"app.request"}, Rules: []dashboard.Rule{},
		Backend: dashboard.Backend{Origin: "http://sample.apps.svc.cluster.local", TokenEnv: "STUDIO_APP_SAMPLE_TOKEN", DeclarationPath: "/declaration", IndexPath: "/index", AssetPath: "/assets/", AssetPrefix: "dashboard/"},
	}}}
	data, err := json.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "registry.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f.service = NewDashboards(f.service.station, path)
	t.Cleanup(f.service.Shutdown)
	return f
}

func TestDashboardInstallationProjectionJoinsOnlyRegisteredInstalledVersions(t *testing.T) {
	f := dashboardListFixture(t)
	f.installations = []dashboardInstallation{f.installation}
	for _, change := range []func(*dashboardInstallation){
		func(i *dashboardInstallation) { i.AppID = "service-without-dashboard" }, func(i *dashboardInstallation) { i.AppVersion = "1.0.1" },
		func(i *dashboardInstallation) { i.State = "pending" }, func(i *dashboardInstallation) { i.State = "failed" }, func(i *dashboardInstallation) { i.State = "removed" },
	} {
		item := f.installation
		item.InstallationID = uuid.Must(uuid.NewV7()).String()
		change(&item)
		f.installations = append(f.installations, item)
	}
	out := f.service.Installations(context.Background(), "test-session")
	var view struct {
		Items []DashboardInstallation `json:"items"`
	}
	if out.Status != 200 || json.Unmarshal(out.Data, &view) != nil || len(view.Items) != 1 || view.Items[0].InstallationID != f.installation.InstallationID {
		t.Fatal("unregistered entry projected", out.Status)
	}
	for _, private := range []string{"svc.cluster.local", "TOKEN", "backend", "artifactDigest"} {
		if strings.Contains(string(out.Data), private) {
			t.Fatal("private configuration projected")
		}
	}
	if f.identityCalls != 1 || f.installationCalls != 1 {
		t.Fatal("projection missed identity or installation")
	}
	f.installations = []dashboardInstallation{}
	out = f.service.Installations(context.Background(), "test-session")
	if out.Status != 200 || string(out.Data) != `{"items":[]}` {
		t.Fatal("empty installation projection changed", out.Status)
	}
}

func TestDashboardInstallationProjectionRejectsForeignAndDuplicateRecords(t *testing.T) {
	for _, kind := range []string{"organization", "station", "duplicate", "revision", "malformed", "denied"} {
		t.Run(kind, func(t *testing.T) {
			f := dashboardListFixture(t)
			f.installations = []dashboardInstallation{f.installation}
			switch kind {
			case "organization":
				f.installations[0].OrganizationID = uuid.Must(uuid.NewV7()).String()
			case "station":
				f.installations[0].StationID = uuid.Must(uuid.NewV7()).String()
			case "duplicate":
				f.installations = append(f.installations, f.installation)
			case "revision":
				f.installations[0].Revision = 0
			case "malformed":
				f.mutatePayload = func(s string) string { return strings.Replace(s, `"revision":3`, `"revision":3,"revision":4`, 1) }
			case "denied":
				f.installationStatus = 403
			}
			if out := f.service.Installations(context.Background(), "test-session"); out.Status < 400 {
				t.Fatal("unsafe listing accepted")
			}
		})
	}
}

func TestDashboardInstallationRejectsActorOnlyAndCrossServiceContexts(t *testing.T) {
	f := newDashboardServiceFixture(t)
	ctx, actor, result := f.service.begin(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatal(result.Status)
	}
	other := NewDashboards(f.service.station, "")
	defer other.Shutdown()
	for _, tc := range []struct {
		service *Dashboards
		ctx     context.Context
		actor   dashboard.Actor
		id      string
	}{
		{f.service, context.Background(), actor, f.installation.InstallationID},
		{other, ctx, actor, f.installation.InstallationID},
		{f.service, ctx, dashboard.Actor{UserID: "other"}, f.installation.InstallationID},
		{f.service, ctx, actor, "../identity/me"},
		{f.service, ctx, actor, f.installation.InstallationID + "?organization=other"},
	} {
		if _, err := tc.service.readInstallation(tc.ctx, tc.actor, tc.id); !errors.Is(err, dashboard.ErrDenied) {
			t.Fatal("invalid request reached Core", err)
		}
	}
	if f.installationCalls != 0 {
		t.Fatal("unsafe context used credentials")
	}
}

func TestDashboardInstallationRejectsForeignMalformedAndFailedResponses(t *testing.T) {
	changes := map[string]func(*dashboardServiceFixture){
		"foreign organization": func(f *dashboardServiceFixture) { f.installation.OrganizationID = uuid.Must(uuid.NewV7()).String() },
		"foreign station":      func(f *dashboardServiceFixture) { f.installation.StationID = uuid.Must(uuid.NewV7()).String() },
		"invalid revision":     func(f *dashboardServiceFixture) { f.installation.Revision = 0 },
		"unknown state":        func(f *dashboardServiceFixture) { f.installation.State = "running" },
		"invalid version":      func(f *dashboardServiceFixture) { f.installation.AppVersion = "latest" },
		"invalid operation":    func(f *dashboardServiceFixture) { f.installation.LastOperationID = "pod-id" },
		"backward timestamps": func(f *dashboardServiceFixture) {
			f.installation.UpdatedAt = f.installation.CreatedAt.Add(-time.Second)
		},
		"forbidden":   func(f *dashboardServiceFixture) { f.installationStatus = 403 },
		"not found":   func(f *dashboardServiceFixture) { f.installationStatus = 404 },
		"unavailable": func(f *dashboardServiceFixture) { f.installationStatus = 503 },
		"redirect":    func(f *dashboardServiceFixture) { f.installationStatus = 302 },
		"duplicate field": func(f *dashboardServiceFixture) {
			f.mutatePayload = func(s string) string { return strings.Replace(s, `"revision":3`, `"revision":3,"revision":4`, 1) }
		},
		"unknown field": func(f *dashboardServiceFixture) {
			f.mutatePayload = func(s string) string { return strings.Replace(s, `"revision":3`, `"revision":3,"trusted":true`, 1) }
		},
		"missing field": func(f *dashboardServiceFixture) {
			f.mutatePayload = func(s string) string { return strings.Replace(s, `"revision":3,`, "", 1) }
		},
		"oversized": func(f *dashboardServiceFixture) {
			f.mutatePayload = func(s string) string { return strings.Repeat(" ", 64<<10) + s }
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			f := newDashboardServiceFixture(t)
			change(f)
			ctx, actor, result := f.service.begin(context.Background(), "test-session")
			if result.Status != 200 {
				t.Fatal(result.Status)
			}
			if _, err := f.service.readInstallation(ctx, actor, f.installation.InstallationID); err == nil {
				t.Fatal("invalid installation accepted")
			}
		})
	}
}

func TestDashboardServiceChecksCurrentPermissionsBeforeOpeningOrClosing(t *testing.T) {
	for _, kind := range []string{"no token", "revoked", "no admin", "no app read"} {
		t.Run(kind, func(t *testing.T) {
			f := newDashboardServiceFixture(t)
			token := "test-session"
			want := 403
			switch kind {
			case "no token":
				token = ""
				want = 401
			case "revoked":
				f.identityStatus = 401
				want = 401
			case "no admin":
				f.identity.Roles = []string{"member"}
			case "no app read":
				f.identity.Scopes = []string{"identity:read", "app:manage"}
			}
			if result := f.service.Open(context.Background(), token, f.installation.InstallationID); result.Status != want {
				t.Fatal(result.Status)
			}
			if _, _, result := f.service.Asset(context.Background(), token, "session", "dashboard/index.html"); result.Status != want {
				t.Fatal(result.Status)
			}
			if result := f.service.Close(context.Background(), token, "session"); result.Status != want {
				t.Fatal(result.Status)
			}
			if result := f.service.Request(context.Background(), token, "session", dashboard.AppRequest{Method: "GET", Path: "/instances"}); result.Status != want {
				t.Fatal(result.Status)
			}
			if f.installationCalls != 0 {
				t.Fatal("unprivileged request read installation")
			}
		})
	}
	f := newDashboardServiceFixture(t)
	if result := f.service.Open(context.Background(), "test-session", f.installation.InstallationID); result.Status != 503 {
		t.Fatal("unconfigured release trusted", result.Status)
	}
	if result := f.service.Close(context.Background(), "test-session", "already-closed"); result.Status != 204 {
		t.Fatal(result.Status)
	}
	if f.installationCalls != 1 {
		t.Fatal("close must not perform installation or instance operation")
	}
}

func TestDashboardInstanceResourceViewJoinsWorkloadMetricsAndStripsBindings(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	station := &Station{base: "https://station.invalid", client: &http.Client{Transport: dashboardIdentityTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/resources/workloads" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		payload := `{"schema_version":2,"updated_at":"` + now + `","gpu_samples":[],"workloads":[{"pod_uid":"e7234653-ce65-49d4-b543-e5acba6510aa","namespace":"verdantflare-comfyui","pod_name":"appr-01a138c4312178d6b1497b10e53edfd4","instance_alias":"","created_at":"` + now + `","containers":[{"name":"worker","cpu":{"value":0.12,"request":0.1,"limit":4,"sampled_at":"` + now + `","quality":"fresh"},"memory":{"value":529858560,"request":268435456,"limit":8589934592,"sampled_at":"` + now + `","quality":"fresh"},"gpu_request":null,"gpu_limit":null}]}]}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}, nil
	})}}
	d := &Dashboards{station: station}
	raw := []byte(`{"schema_version":1,"allowed_actions":["create"],"profiles":[{"profile_id":"base-cpu","name":"基础 CPU"}],"instances":[{"instance_id":"019d8b35-02fd-778e-a4a0-a1f9df750c3d","alias":"comfyui019d8b3502fd778ea4a0a1f9df750c3d","name":"ComfyUI Dashboard A","status":"running","observed_at":"` + now + `","access":"manage","allowed_actions":["open","stop"],"editor_state":"ready","package_profile":"base","resources":{},"_resource_binding":{"pod_uid":"e7234653-ce65-49d4-b543-e5acba6510aa","namespace":"verdantflare-comfyui","pod_name":"appr-01a138c4312178d6b1497b10e53edfd4","container":"worker"}}]}`)
	enriched, ok := d.instanceResourceView(context.Background(), "test-session", raw)
	if !ok {
		t.Fatal("expected instanceResourceView to succeed")
	}
	if strings.Contains(string(enriched), "_resource_binding") {
		t.Fatal("private _resource_binding leaked")
	}
	var parsed struct {
		AllowedActions []string `json:"allowed_actions"`
		Instances      []struct {
			Resources map[string]dashboardResourceMetric `json:"resources"`
		} `json:"instances"`
	}
	if err := json.Unmarshal(enriched, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.AllowedActions) != 1 || parsed.AllowedActions[0] != "create" {
		t.Fatalf("unexpected allowed_actions: %v", parsed.AllowedActions)
	}
	res := parsed.Instances[0].Resources
	if res["cpu"].Quality != "fresh" || res["cpu"].Value == nil || *res["cpu"].Value != 0.12 {
		t.Fatalf("unexpected cpu metric: %+v", res["cpu"])
	}
	if res["memory"].Quality != "fresh" || res["memory"].Value == nil || *res["memory"].Value != 529858560 {
		t.Fatalf("unexpected memory metric: %+v", res["memory"])
	}
	if res["gpu"].Quality != "fresh" || res["gpu"].Value == nil || *res["gpu"].Value != 0 {
		t.Fatalf("unexpected gpu metric: %+v", res["gpu"])
	}
}

