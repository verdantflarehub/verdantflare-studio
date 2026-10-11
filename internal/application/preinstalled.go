package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type Preinstalled struct {
	station    *Station
	configPath string
	store      preinstalled.Store
}

var preinstalledSHA = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func NewPreinstalled(station *Station, configPath string, store preinstalled.Store) *Preinstalled {
	return &Preinstalled{station, configPath, store}
}
func (p *Preinstalled) Get(ctx context.Context, token string) Result {
	return p.run(ctx, token, preinstalled.Intent{ID: uuid.Must(uuid.NewV7()).String()})
}
func (p *Preinstalled) Execute(ctx context.Context, token string, in preinstalled.Intent) Result {
	in.Execute = true
	return p.run(ctx, token, in)
}
func preinstalledError(err error) Result {
	status, code := 503, "PREINSTALLED_DEPENDENCY_UNAVAILABLE"
	if errors.Is(err, preinstalled.ErrDenied) {
		status, code = 403, "PREINSTALLED_DENIED"
	} else if errors.Is(err, preinstalled.ErrInvalid) {
		status, code = 400, "INVALID_PREINSTALLED_REQUEST"
	} else if errors.Is(err, preinstalled.ErrConflict) {
		status, code = 409, "PREINSTALLED_STATE_CONFLICT"
	}
	return Error(status, code)
}
func (p *Preinstalled) run(ctx context.Context, token string, in preinstalled.Intent) Result {
	if p == nil || p.station == nil {
		return preinstalledError(preinstalled.ErrUnavailable)
	}
	id := uuid.Must(uuid.NewV7()).String()
	identity, status := p.station.verifiedIdentity(ctx, token, id)
	if status != 200 {
		return Error(status, "PREINSTALLED_IDENTITY_UNAVAILABLE")
	}
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") || in.Execute && !slices.Contains(identity.Scopes, "app:manage") {
		return preinstalledError(preinstalled.ErrDenied)
	}
	if p.configPath == "" {
		raw, _ := json.Marshal(map[string]any{"enabled": false, "configDigest": "", "items": []any{}})
		return Result{Status: 200, RequestID: id, Data: raw}
	}
	config, err := preinstalled.LoadConfig(p.configPath)
	if err != nil {
		return preinstalledError(err)
	}
	actor := preinstalled.Actor{UserID: identity.UserID, OrganizationID: identity.OrganizationID, StationID: identity.StationID, Roles: identity.Roles, Scopes: identity.Scopes}
	backend := &preinstalledStation{station: p.station, token: token, actor: actor, bindings: map[string]preinstalledRelease{}}
	coordinator := preinstalled.Reconciler{Lifecycle: backend, Store: p.store}
	out, err := coordinator.Reconcile(ctx, actor, config, in)
	if err != nil {
		return preinstalledError(err)
	}
	raw, _ := json.Marshal(struct {
		Enabled bool `json:"enabled"`
		preinstalled.Result
	}{true, out})
	return Result{Status: 200, RequestID: id, Data: raw}
}

type preinstalledRelease struct {
	ReleaseRef     string `json:"releaseRef"`
	AppID          string `json:"appId"`
	AppVersion     string `json:"appVersion"`
	PackageDigest  string `json:"packageDigest"`
	ChartDigest    string `json:"chartDigest"`
	TemplateDigest string `json:"templateDigest"`
}
type preinstalledStation struct {
	station  *Station
	token    string
	actor    preinstalled.Actor
	bindings map[string]preinstalledRelease
}

func (b *preinstalledStation) call(ctx context.Context, method, path string, body any, out any) error {
	var raw []byte
	var err error
	if body != nil {
		raw, err = json.Marshal(body)
		if err != nil {
			return preinstalled.ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, b.station.base+path, bytes.NewReader(raw))
	if err != nil {
		return preinstalled.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", uuid.Must(uuid.NewV7()).String())
	response, err := b.station.client.Do(req)
	if err != nil {
		return preinstalled.ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return preinstalled.ErrDenied
	}
	if response.StatusCode == 409 {
		return preinstalled.ErrConflict
	}
	if response.StatusCode != 200 && response.StatusCode != 202 {
		return preinstalled.ErrUnavailable
	}
	raw, err = strictjson.ReadValue(response.Body, 2<<20)
	if err != nil {
		return preinstalled.ErrUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return preinstalled.ErrUnavailable
	}
	canonical, err := json.Marshal(out)
	if err != nil {
		return preinstalled.ErrUnavailable
	}
	var original, normalized any
	if json.Unmarshal(raw, &original) != nil || json.Unmarshal(canonical, &normalized) != nil || !preinstalledShape(original, normalized) {
		return preinstalled.ErrUnavailable
	}
	return nil
}
func preinstalledShape(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !preinstalledShape(v, w) {
				return false
			}
		}
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !preinstalledShape(x[i], y[i]) {
				return false
			}
		}
	case string:
		_, ok := b.(string)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	case float64:
		_, ok := b.(float64)
		return ok
	case nil:
		return b == nil
	default:
		return false
	}
	return true
}
func (b *preinstalledStation) sameActor(a preinstalled.Actor) bool {
	return a.UserID == b.actor.UserID && a.OrganizationID == b.actor.OrganizationID && a.StationID == b.actor.StationID
}
func (b *preinstalledStation) Verify(ctx context.Context, actor preinstalled.Actor, release preinstalled.Release) error {
	if actor.UserID != b.actor.UserID || actor.OrganizationID != b.actor.OrganizationID || actor.StationID != b.actor.StationID {
		return preinstalled.ErrDenied
	}
	var binding preinstalledRelease
	if err := b.call(ctx, "GET", "/preinstalled/releases/"+release.ReleaseRef, nil, &binding); err != nil {
		return err
	}
	if binding.ReleaseRef != release.ReleaseRef || binding.AppID != release.AppID || binding.AppVersion != release.AppVersion || binding.PackageDigest != release.PackageDigest || binding.ChartDigest != release.ChartDigest || !preinstalledSHA.MatchString(binding.TemplateDigest) {
		return preinstalled.ErrUnavailable
	}
	b.bindings[release.AppID] = binding
	return nil
}
func (b *preinstalledStation) Observe(ctx context.Context, actor preinstalled.Actor, app string) (preinstalled.Inventory, error) {
	if !b.sameActor(actor) {
		return preinstalled.Inventory{}, preinstalled.ErrDenied
	}
	var payload struct {
		AppID            string `json:"appId"`
		Revision         string `json:"revision"`
		Complete         bool   `json:"complete"`
		WorkloadUID      string `json:"workloadUid"`
		ManagementStatus string `json:"managementStatus"`
		ObservedAt       string `json:"observedAt"`
		History          []struct {
			Installation dashboardInstallation `json:"installation"`
			Release      *preinstalledRelease  `json:"release"`
			Confirmed    bool                  `json:"confirmed"`
		} `json:"history"`
	}
	err := b.call(ctx, "GET", "/preinstalled/apps/"+app, nil, &payload)
	if err != nil {
		return preinstalled.Inventory{}, err
	}
	observed, timeErr := time.Parse(time.RFC3339Nano, payload.ObservedAt)
	if payload.AppID != app || !preinstalledSHA.MatchString(payload.Revision) || payload.History == nil || len(payload.History) > 1000 || payload.Complete && (timeErr != nil || observed.IsZero()) {
		return preinstalled.Inventory{}, preinstalled.ErrUnavailable
	}
	if payload.WorkloadUID != "" {
		uid, e := uuid.Parse(payload.WorkloadUID)
		if e != nil || uid == uuid.Nil || uid.String() != payload.WorkloadUID || !slices.Contains([]string{"unknown", "unmanaged", "managed", "foreign"}, payload.ManagementStatus) {
			return preinstalled.Inventory{}, preinstalled.ErrUnavailable
		}
	}
	binding, ok := b.bindings[app]
	if !ok {
		return preinstalled.Inventory{}, preinstalled.ErrUnavailable
	}
	out := preinstalled.Inventory{Revision: payload.Revision, Complete: payload.Complete, WorkloadUID: payload.WorkloadUID, ManagementStatus: payload.ManagementStatus, ObservedAt: payload.ObservedAt, TemplateDigest: binding.TemplateDigest}
	seen := map[string]bool{}
	for _, record := range payload.History {
		i := record.Installation
		id, err := uuid.Parse(i.InstallationID)
		if err != nil || id.Version() != 7 || id.String() != i.InstallationID || !validDashboardInstallation(i) || i.AppID != app || i.OrganizationID != actor.OrganizationID || i.StationID != actor.StationID || seen[i.InstallationID] {
			return preinstalled.Inventory{}, preinstalled.ErrUnavailable
		}
		seen[i.InstallationID] = true
		if record.Confirmed && record.Release == nil {
			return preinstalled.Inventory{}, preinstalled.ErrUnavailable
		}
		if record.Release != nil && (record.Release.AppID != app || record.Release.AppVersion != i.AppVersion || !preinstalledSHA.MatchString(record.Release.PackageDigest) || !preinstalledSHA.MatchString(record.Release.ChartDigest) || !preinstalledSHA.MatchString(record.Release.TemplateDigest)) {
			return preinstalled.Inventory{}, preinstalled.ErrUnavailable
		}
		if i.State == "removed" {
			out.RemovedInstallationID = i.InstallationID
			continue
		}
		if out.Current != nil {
			return preinstalled.Inventory{}, preinstalled.ErrUnavailable
		}
		out.Current = &preinstalled.Fact{InstallationID: i.InstallationID, State: i.State, AppVersion: i.AppVersion, LastOperationID: i.LastOperationID, Confirmed: record.Confirmed}
		if record.Release != nil {
			if record.Release.AppID != app || record.Release.AppVersion != i.AppVersion {
				return preinstalled.Inventory{}, preinstalled.ErrUnavailable
			}
			out.Current.PackageDigest = record.Release.PackageDigest
			out.Current.ChartDigest = record.Release.ChartDigest
			out.Current.TemplateDigest = record.Release.TemplateDigest
		}
	}
	return out, nil
}

type preinstalledOperation struct {
	RequestID      string          `json:"request_id"`
	OperationID    string          `json:"operation_id"`
	OrganizationID string          `json:"organization_id"`
	StationID      string          `json:"station_id"`
	AppID          string          `json:"app_id"`
	AppVersion     string          `json:"app_version"`
	Action         string          `json:"action"`
	Status         string          `json:"status"`
	Phase          string          `json:"phase"`
	Download       json.RawMessage `json:"download"`
	Error          json.RawMessage `json:"error"`
	UpdatedAt      string          `json:"updated_at"`
}

func (b *preinstalledStation) operation(ctx context.Context, method, path string, body any) (preinstalled.Operation, error) {
	var out preinstalledOperation
	if err := b.call(ctx, method, path, body, &out); err != nil {
		return preinstalled.Operation{}, err
	}
	if out.OrganizationID != b.actor.OrganizationID || out.StationID != b.actor.StationID || !slices.Contains([]string{"install", "adopt"}, out.Action) {
		return preinstalled.Operation{}, preinstalled.ErrUnavailable
	}
	code := ""
	if out.Status == "failed" {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(out.Error, &failure)
		code = failure.Code
	}
	return preinstalled.Operation{ID: out.OperationID, AppID: out.AppID, AppVersion: out.AppVersion, Status: out.Status, Code: code}, nil
}
func (b *preinstalledStation) Operation(ctx context.Context, actor preinstalled.Actor, id string) (preinstalled.Operation, error) {
	if !b.sameActor(actor) {
		return preinstalled.Operation{}, preinstalled.ErrDenied
	}
	if _, err := uuid.Parse(id); err != nil {
		return preinstalled.Operation{}, preinstalled.ErrInvalid
	}
	out, err := b.operation(ctx, "GET", "/app-operations/"+id, nil)
	if err == nil && out.ID != id {
		return preinstalled.Operation{}, preinstalled.ErrUnavailable
	}
	return out, err
}
func (b *preinstalledStation) Submit(ctx context.Context, in preinstalled.Command) (preinstalled.Operation, error) {
	binding, ok := b.bindings[in.Release.AppID]
	if !ok || binding.ReleaseRef != in.Release.ReleaseRef || binding.PackageDigest != in.Release.PackageDigest || binding.ChartDigest != in.Release.ChartDigest || binding.TemplateDigest != in.TemplateDigest || in.Actor.UserID != b.actor.UserID || in.Actor.OrganizationID != b.actor.OrganizationID || in.Actor.StationID != b.actor.StationID {
		return preinstalled.Operation{}, preinstalled.ErrUnavailable
	}
	command := map[string]any{"request_id": in.RequestID, "idempotency_key": in.IdempotencyKey, "organization_id": b.actor.OrganizationID, "station_id": b.actor.StationID, "app_id": in.Release.AppID, "app_version": in.Release.AppVersion, "action": in.Mode}
	if in.ExpectedWorkloadUID != "" {
		command["expected_workload_uid"] = in.ExpectedWorkloadUID
	}
	return b.operation(ctx, "POST", "/preinstalled/commands", map[string]any{"command": command, "release": binding, "inventoryRevision": in.InventoryRevision, "reinstallRemovedId": in.ReinstallRemovedID})
}
