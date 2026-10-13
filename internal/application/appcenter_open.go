package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

var errAppCenterBindingChanged = errors.New("APPCENTER_BINDING_CHANGED")

func (a *AppCenter) Open(ctx context.Context, token string, request appcenter.OpenRequest) Result {
	id := uuid.Must(uuid.NewV7()).String()
	fail := func(status int, code string) Result { return appCenterError(status, code, id) }
	raw, err := json.Marshal(request)
	var checked appcenter.OpenRequest
	if err != nil || appcenter.Decode(bytes.NewReader(raw), "open", 4<<10, &checked) != nil {
		return fail(400, "INVALID_APPCENTER_REQUEST")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	view, metadata, result := a.read(ctx, token)
	if result.Status != 200 {
		return result
	}
	id = result.RequestID
	var target *appcenter.Card
	for i := range view.Items {
		if view.Items[i].CardKey == request.CardKey {
			target = &view.Items[i]
			break
		}
	}
	if target == nil {
		return fail(409, "APPCENTER_BINDING_CHANGED")
	}
	// The full view revision is diagnostic only: unrelated cards may have changed.
	if target.Entry == nil || target.Entry.EntryID != request.EntryID {
		return fail(409, "APPCENTER_BINDING_CHANGED")
	}
	if target.Entry.Availability == "denied" {
		return fail(403, "PERMISSION_DENIED")
	}
	if target.Entry.Availability == "incompatible" {
		return fail(409, "HOST_UNSUPPORTED")
	}
	enabled := false
	for _, action := range target.Actions {
		if action.Kind == "open" && action.Enabled && action.EntryID != nil && *action.EntryID == request.EntryID {
			enabled = true
		}
	}
	if !enabled {
		return fail(409, "APPCENTER_OPEN_UNAVAILABLE")
	}
	if target.Entry.Transport == "legacy" {
		binding := metadata[target.CardKey].Legacy
		if binding == nil {
			return fail(409, "APPCENTER_BINDING_CHANGED")
		}
		data, _ := json.Marshal(appcenter.LegacyOpened{SchemaVersion: 1, Transport: "legacy", AdapterID: binding.AdapterID, BindingID: binding.BindingID, BindingRevision: target.BindingRevision, Route: binding.Route})
		return Result{Status: 200, Data: data, RequestID: id}
	}
	if a.dashboards == nil || target.Entry.Transport != "dashboard" || target.Installation == nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	ctx, actor, auth := a.dashboards.begin(ctx, token)
	if auth.Status != 200 {
		code := "APPCENTER_DEPENDENCY_UNAVAILABLE"
		if auth.Status == 401 {
			code = "UNAUTHENTICATED"
		}
		if auth.Status == 403 {
			code = "PERMISSION_DENIED"
		}
		return fail(auth.Status, code)
	}
	if (appcenter.Scope{UserID: actor.UserID, OrganizationID: actor.OrganizationID, StationID: actor.StationID, AuthorizationEpoch: actor.AuthorizationEpoch}) != view.Scope {
		return fail(409, "APPCENTER_BINDING_CHANGED")
	}
	launch, err := a.dashboards.resolver.OpenChecked(ctx, actor, target.Installation.InstallationID, func(binding dashboard.Binding) error {
		if binding.AppID != target.AppID || binding.AppVersion != target.Installation.AppVersion {
			return errAppCenterBindingChanged
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errAppCenterBindingChanged) {
			return fail(409, "APPCENTER_BINDING_CHANGED")
		}
		out := dashboardError(err, id)
		var payload struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(out.Data, &payload) != nil || payload.Code == "" {
			return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		return fail(out.Status, payload.Code)
	}
	data, _ := json.Marshal(struct {
		SchemaVersion int             `json:"schemaVersion"`
		Transport     string          `json:"transport"`
		Opened        json.RawMessage `json:"opened"`
	}{1, "dashboard", dashboardOpened(actor, launch)})
	return Result{Status: 200, Data: data, RequestID: id}
}
