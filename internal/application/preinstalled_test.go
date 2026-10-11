package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
)

func TestPreinstalledStationUsesScopedClosedInventoryAndFixedEndpoint(t *testing.T) {
	i := dashboardIdentityFixture()
	actor := preinstalled.Actor{UserID: i.UserID, OrganizationID: i.OrganizationID, StationID: i.StationID, Roles: []string{"admin"}, Scopes: []string{"app:read", "app:manage"}}
	binding := preinstalledRelease{"resources-v1", "resources", "0.1.0", "sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64), "sha256:" + strings.Repeat("c", 64)}
	release := preinstalled.Release{AppID: binding.AppID, AppVersion: binding.AppVersion, ReleaseRef: binding.ReleaseRef, PackageDigest: binding.PackageDigest, ChartDigest: binding.ChartDigest, Dependencies: []string{}}
	removedID, workloadID, opID := uuid.Must(uuid.NewV7()).String(), uuid.NewString(), uuid.Must(uuid.NewV7()).String()
	history := dashboardInstallation{InstallationID: removedID, AppID: binding.AppID, AppVersion: binding.AppVersion, StationID: actor.StationID, OrganizationID: actor.OrganizationID, State: "removed", Revision: 2, LastOperationID: opID, CreatedAt: time.Now().Add(-time.Hour).UTC(), UpdatedAt: time.Now().UTC()}
	var mutate func(map[string]any)
	postStatus, posts := 202, 0
	station := &Station{base: "https://station.invalid", client: &http.Client{Transport: dashboardIdentityTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer controlled-session" || req.Header.Get("Cookie") != "" || req.URL.Host != "station.invalid" || req.URL.RawQuery != "" {
			t.Fatal("uncontrolled request")
		}
		var payload any
		status := 200
		switch req.URL.Path {
		case "/preinstalled/releases/resources-v1":
			payload = binding
		case "/preinstalled/apps/resources":
			view := map[string]any{"appId": "resources", "revision": "sha256:" + strings.Repeat("d", 64), "complete": true, "workloadUid": workloadID, "managementStatus": "unmanaged", "observedAt": "2026-10-11T00:00:00Z", "history": []any{map[string]any{"installation": history, "release": nil, "confirmed": false}}}
			if mutate != nil {
				mutate(view)
			}
			payload = view
		case "/preinstalled/commands":
			if req.Method != "POST" {
				t.Fatal("incorrect command method")
			}
			posts++
			var in struct {
				Command  map[string]string   `json:"command"`
				Release  preinstalledRelease `json:"release"`
				Revision string              `json:"inventoryRevision"`
				Removed  string              `json:"reinstallRemovedId"`
			}
			if json.NewDecoder(req.Body).Decode(&in) != nil || in.Release != binding || in.Removed != removedID || in.Command["organization_id"] != actor.OrganizationID || in.Command["station_id"] != actor.StationID || in.Command["expected_workload_uid"] != workloadID {
				t.Fatal("trusted command evidence lost")
			}
			status = postStatus
			payload = preinstalledOperation{RequestID: in.Command["request_id"], OperationID: opID, OrganizationID: actor.OrganizationID, StationID: actor.StationID, AppID: "resources", AppVersion: "0.1.0", Action: "adopt", Status: "accepted", Phase: "checking", Download: json.RawMessage(`{"downloaded_bytes":0,"total_bytes":null,"percent":null}`), Error: json.RawMessage(`null`), UpdatedAt: "2026-10-11T00:00:00Z"}
		default:
			t.Fatal("legacy or uncontrolled endpoint used", req.URL.Path)
		}
		raw, _ := json.Marshal(payload)
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(raw))), Header: http.Header{"Content-Type": {"application/json"}}, Request: req}, nil
	})}}
	b := &preinstalledStation{station: station, token: "controlled-session", actor: actor, bindings: map[string]preinstalledRelease{}}
	ctx := context.Background()
	if err := b.Verify(ctx, actor, release); err != nil {
		t.Fatal(err)
	}
	facts, err := b.Observe(ctx, actor, "resources")
	if err != nil || facts.RemovedInstallationID != removedID || facts.Current != nil || facts.WorkloadUID != workloadID || facts.TemplateDigest != binding.TemplateDigest {
		t.Fatal(facts, err)
	}
	for _, kind := range []string{"scope", "missing-complete", "case-alias", "false-confirmation"} {
		t.Run(kind, func(t *testing.T) {
			mutate = func(v map[string]any) {
				switch kind {
				case "scope":
					foreign := history
					foreign.OrganizationID = uuid.NewString()
					v["history"] = []any{map[string]any{"installation": foreign, "release": nil, "confirmed": false}}
				case "missing-complete":
					delete(v, "complete")
				case "case-alias":
					v["Complete"] = v["complete"]
					delete(v, "complete")
				case "false-confirmation":
					v["history"] = []any{map[string]any{"installation": history, "release": nil, "confirmed": true}}
				}
			}
			if _, err := b.Observe(ctx, actor, "resources"); err == nil {
				t.Fatal("invalid inventory accepted")
			}
		})
	}
	mutate = nil
	command := preinstalled.Command{Actor: actor, Release: release, RequestID: uuid.Must(uuid.NewV7()).String(), IdempotencyKey: "original-intent-key", InventoryRevision: facts.Revision, Mode: "adopt", ExpectedWorkloadUID: workloadID, ReinstallRemovedID: removedID, TemplateDigest: binding.TemplateDigest}
	if out, err := b.Submit(ctx, command); err != nil || out.ID != opID {
		t.Fatal(out, err)
	}
	postStatus = 409
	if _, err := b.Submit(ctx, command); !errors.Is(err, preinstalled.ErrConflict) || posts != 2 {
		t.Fatal("conflict retried through fallback", err, posts)
	}
	command.TemplateDigest = "sha256:" + strings.Repeat("e", 64)
	if _, err := b.Submit(ctx, command); err == nil || posts != 2 {
		t.Fatal("changed binding submitted")
	}
}

func TestPreinstalledDisabledStillRequiresCurrentIdentity(t *testing.T) {
	f := newDashboardServiceFixture(t)
	p := NewPreinstalled(f.service.station, "", nil)
	result := p.Get(context.Background(), "test-session")
	if result.Status != 200 || !strings.Contains(string(result.Data), `"enabled":false`) || f.identityCalls != 1 || f.installationCalls != 0 {
		t.Fatal(result.Status, string(result.Data))
	}
	f.identityStatus = 401
	if result = p.Get(context.Background(), "test-session"); result.Status != 401 || f.identityCalls != 2 {
		t.Fatal("identity was cached", result.Status)
	}
}
