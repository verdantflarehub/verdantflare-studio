package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

func commandTestApp(t *testing.T) (*AppCenter, *appcenter.Facts, *identityContext, *[]appcenter.CommandCore, *string) {
	t.Helper()
	raw, err := os.ReadFile("../appcenter/testdata/rules.generated.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name  string          `json:"name"`
		Facts appcenter.Facts `json:"facts"`
	}
	if json.Unmarshal(raw, &fixtures) != nil {
		t.Fatal("fixtures")
	}
	var facts appcenter.Facts
	for _, f := range fixtures {
		if f.Name == "verified-candidate-install" {
			facts = f.Facts
		}
	}
	now := time.Now().UTC()
	facts.GeneratedAt = now
	facts.Items[0].Service.ObservedAt = &now
	identity := identityContext{UserID: facts.Scope.UserID, OrganizationID: facts.Scope.OrganizationID, StationID: facts.Scope.StationID,
		SessionID: "00000000-0000-7000-8000-000000000009", Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"},
		Scopes: []string{"identity:read", "app:read", "app:manage"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
	calls, mode := []appcenter.CommandCore{}, ""
	s := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("auth")
		}
		var body []byte
		status := 200
		switch r.URL.Path {
		case "/identity/me":
			identity.RequestID = r.Header.Get("X-Request-ID")
			body, _ = json.Marshal(identity)
		case "/appcenter/v1/facts":
			if strings.HasPrefix(mode, "submit") {
				t.Fatal("submit read current facts")
			}
			facts.RequestID = r.Header.Get("X-Request-ID")
			facts.Scope.AuthorizationEpoch = dashboardActorFromIdentity(identity).AuthorizationEpoch
			body, _ = json.Marshal(facts)
		case "/appcenter/v1/commands":
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
				t.Fatal("method or media")
			}
			var in appcenter.CommandCore
			if appcenter.Decode(r.Body, "command-core", 16<<10, &in) != nil {
				t.Fatal("invalid forwarding")
			}
			calls = append(calls, in)
			if mode == "submit-lost" {
				return nil, errors.New("response lost")
			}
			status = 202
			body, _ = json.Marshal(map[string]any{"request_id": "original-trace", "operation_id": "00000000-0000-7000-8000-000000000010",
				"organization_id": in.Command.OrganizationID, "station_id": in.Command.StationID, "app_id": in.Command.AppID,
				"app_version": in.Command.AppVersion, "action": in.Command.Action, "status": "accepted", "phase": "checking",
				"download": map[string]any{"downloaded_bytes": 0, "total_bytes": nil, "percent": nil}, "error": nil, "updated_at": now})
			if mode == "submit-foreign" {
				body = bytes.Replace(body, []byte(`"app_id":"sample"`), []byte(`"app_id":"other"`), 1)
			}
			if mode == "submit-duplicate" {
				body = bytes.Replace(body, []byte(`"status":"accepted"`), []byte(`"status":"accepted","status":"accepted"`), 1)
			}
			if mode == "submit-conflict" {
				status = 409
				body, _ = json.Marshal(map[string]string{"code": "CONFLICT", "requestId": r.Header.Get("X-Request-ID")})
			}
		default:
			t.Fatal("unexpected path")
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}}
	a := NewAppCenter(s, "")
	a.now = func() time.Time { return now }
	return a, &facts, &identity, &calls, &mode
}

func prepareTestCommand(t *testing.T, a *AppCenter) (appcenter.CommandPrepare, appcenter.CommandCore) {
	t.Helper()
	view, _, result := a.read(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatalf("view status %d", result.Status)
	}
	in := appcenter.CommandPrepare{SchemaVersion: 1, RequestID: "client-original", IdempotencyKey: "client-fixed-key", CardKey: view.Items[0].CardKey, ActionID: "install", ExpectedBindingRevision: view.Items[0].BindingRevision}
	out := a.Prepare(context.Background(), "test-session", in)
	var prepared appcenter.CommandCore
	if out.Status != 200 || appcenter.Decode(bytes.NewReader(out.Data), "command-core", 16<<10, &prepared) != nil {
		t.Fatalf("prepare status %d: %s", out.Status, out.Data)
	}
	return in, prepared
}

func TestAppCenterPrepareSnapshotAndPermissions(t *testing.T) {
	a, facts, identity, calls, _ := commandTestApp(t)
	in, prepared := prepareTestCommand(t, a)
	if len(*calls) != 0 || prepared.Command.RequestID != in.RequestID || prepared.Command.IdempotencyKey != in.IdempotencyKey ||
		prepared.Expected.InstallationID != nil || prepared.Expected.InstallationRevision != nil || prepared.Expected.WorkloadUID != nil ||
		*prepared.Expected.ReleaseDigest != *facts.Items[0].Lifecycle.Candidates[0].ReleaseDigest {
		t.Fatal("not a read-only fixed snapshot")
	}
	facts.Items[0].Lifecycle.PolicyRevision = "sha256:" + strings.Repeat("b", 64)
	if out := a.Prepare(context.Background(), "test-session", in); out.Status != 409 {
		t.Fatalf("stale binding accepted: %d", out.Status)
	}
	identity.Scopes = []string{"identity:read", "app:read"}
	if out := a.Prepare(context.Background(), "test-session", in); out.Status != 403 {
		t.Fatal("manage permission bypassed")
	}
}

func TestAppCenterPrepareInstalledVersionAndUID(t *testing.T) {
	a, facts, _, _, _ := commandTestApp(t)
	fact := &facts.Items[0]
	fact.Origin = "installation"
	fact.Installation = &appcenter.Installation{InstallationID: "00000000-0000-7000-8000-000000000099", Revision: 7, AppVersion: "1.0.0", State: "installed"}
	fact.CardKey = "installation:" + fact.Installation.InstallationID
	fact.Service.State = "stopped"
	fact.Service.WorkloadUID = appcenter.Text("00000000-0000-4000-8000-000000000088")
	var catalog map[string]any
	if json.Unmarshal(fact.Catalog, &catalog) != nil {
		t.Fatal("catalog fixture")
	}
	catalog["version"] = "2.0.0"
	fact.Catalog, _ = json.Marshal(catalog)
	fact.Lifecycle.Candidates[0].Kind = "start"
	fact.Lifecycle.Candidates[0].ExpectedWorkloadUID = fact.Service.WorkloadUID
	view, _, result := a.read(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatal("view unavailable")
	}
	if view.Items[0].CandidateVersion == nil || *view.Items[0].CandidateVersion != "2.0.0" {
		t.Fatal("newer catalog fixture absent")
	}
	out := a.Prepare(context.Background(), "test-session", appcenter.CommandPrepare{SchemaVersion: 1, RequestID: "fixed-trace", IdempotencyKey: "fixed-start-key", CardKey: fact.CardKey, ActionID: "start", ExpectedBindingRevision: view.Items[0].BindingRevision})
	var prepared appcenter.CommandCore
	if out.Status != 200 || json.Unmarshal(out.Data, &prepared) != nil {
		t.Fatalf("prepare: %d %s", out.Status, out.Data)
	}
	if prepared.Command.AppVersion != "1.0.0" || prepared.Expected.InstallationID == nil || *prepared.Expected.InstallationID != fact.Installation.InstallationID || prepared.Expected.InstallationRevision == nil || *prepared.Expected.InstallationRevision != 7 || prepared.Expected.WorkloadUID == nil || *prepared.Expected.WorkloadUID != *fact.Service.WorkloadUID {
		t.Fatal("installed version/identity not preserved")
	}
}

func TestAppCenterSubmitFixedReplayBoundary(t *testing.T) {
	for _, scenario := range []string{"submit", "submit-lost", "submit-foreign", "submit-duplicate", "submit-conflict", "scope", "permission"} {
		t.Run(scenario, func(t *testing.T) {
			a, _, identity, calls, mode := commandTestApp(t)
			_, prepared := prepareTestCommand(t, a)
			*mode = scenario
			// Invalid presentation cannot prevent submission/recovery.
			a.presentationPath = "missing-presentation.json"
			if scenario == "scope" {
				identity.OrganizationID = "00000000-0000-7000-8000-000000000077"
			}
			if scenario == "permission" {
				identity.Scopes = []string{"identity:read", "app:read"}
			}
			out := a.Submit(context.Background(), "test-session", appcenter.CommandSubmit{SchemaVersion: 1, Prepared: prepared})
			want := map[string]int{"submit": 202, "submit-lost": 503, "submit-foreign": 502, "submit-duplicate": 502, "submit-conflict": 409, "scope": 403, "permission": 403}[scenario]
			if out.Status != want {
				t.Fatalf("status %d want %d: %s", out.Status, want, out.Data)
			}
			if want == 403 {
				if len(*calls) != 0 {
					t.Fatal("scope escaped")
				}
				return
			}
			if len(*calls) != 1 || !reflect.DeepEqual((*calls)[0], prepared) {
				t.Fatal("fixed input changed")
			}
			if scenario == "submit" && !bytes.Contains(out.Data, []byte(`"request_id":"original-trace"`)) {
				t.Fatal("original trace lost")
			}
			if scenario == "submit-lost" {
				*mode = "submit"
				replay := a.Submit(context.Background(), "test-session", appcenter.CommandSubmit{SchemaVersion: 1, Prepared: prepared})
				if replay.Status != 202 || len(*calls) != 2 || !reflect.DeepEqual((*calls)[0], (*calls)[1]) {
					t.Fatal("replay changed original input")
				}
			}
		})
	}
}
