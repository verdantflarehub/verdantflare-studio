package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type Navigation struct {
	station     *Station
	source      navigation.Source
	preferences navigation.PreferenceStore
	close       func()
	appCenter   *AppCenter
}

func NewNavigation(station *Station, registryPath, dashboardRegistryPath string, store navigation.PreferenceStore) *Navigation {
	metadata := dashboard.NewRegisteredSource(dashboardRegistryPath, nil)
	return &Navigation{station: station, source: &navigation.RegisteredSource{Path: registryPath, Artifacts: metadata.RegisteredArtifacts}, preferences: store, close: metadata.Close}
}

// WithAppCenter binds the same authenticated projection as the directory.
// This is startup wiring; legacy registry mode still has no App entries.
func (n *Navigation) WithAppCenter(a *AppCenter) *Navigation {
	n.appCenter = a
	return n
}

func (n *Navigation) Shutdown() {
	if n.close != nil {
		n.close()
	}
}

type NavigationResult struct {
	Mode               string                `json:"mode"`
	Projection         navigation.Projection `json:"projection"`
	PreferenceRevision string                `json:"preferenceRevision"`
}

type NavigationPreferencePatch struct {
	AppID              string `json:"appId"`
	EntryID            string `json:"entryId"`
	Pinned             bool   `json:"pinned"`
	Order              int    `json:"order"`
	PreferenceRevision string `json:"preferenceRevision"`
}

func navigationError(err error, requestID string) Result {
	status, code := 503, "NAVIGATION_UNAVAILABLE"
	switch {
	case errors.Is(err, navigation.ErrInvalid):
		status, code = 502, "INVALID_NAVIGATION_BINDING"
	case errors.Is(err, navigation.ErrConflict):
		status, code = 409, "NAVIGATION_PREFERENCE_CONFLICT"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 408, "NAVIGATION_REQUEST_CANCELED"
	}
	out := Error(status, code)
	out.RequestID = requestID
	return out
}

type navigationInput struct {
	navigation.Input
	Mode string
}

func (n *Navigation) input(ctx context.Context, token string) (navigationInput, string, Result) {
	var in navigationInput
	requestID := uuid.Must(uuid.NewV7()).String()
	if n.station == nil || n.source == nil {
		return in, "", navigationError(navigation.ErrUnavailable, requestID)
	}
	identity, status := n.station.verifiedIdentity(ctx, token, requestID)
	if status != 200 {
		code := "NAVIGATION_UNAVAILABLE"
		if status == 401 {
			code = "UNAUTHENTICATED"
		} else if status == 403 {
			code = "NAVIGATION_DENIED"
		}
		out := Error(status, code)
		out.RequestID = requestID
		return in, "", out
	}
	actor := dashboardActorFromIdentity(identity)
	in.Scope = navigation.Scope{UserID: actor.UserID, OrganizationID: actor.OrganizationID, StationID: actor.StationID, AuthorizationEpoch: actor.AuthorizationEpoch}
	in.Roles, in.Scopes = identity.Roles, identity.Scopes
	catalog, err := n.source.Load(ctx)
	if err != nil {
		return in, "", navigationError(err, requestID)
	}
	in.Defaults, in.Rules = catalog.Defaults, catalog.Rules
	in.Mode = catalog.Mode
	if in.Mode == "legacy" {
		in.Preferences = navigation.EmptyPreferences(in.Scope)
		in.Defaults, in.Rules = nil, nil
		return in, "0", Result{Status: 200, RequestID: requestID}
	}
	if in.Mode != "preinstalled" || n.preferences == nil {
		return in, "", navigationError(navigation.ErrUnavailable, requestID)
	}
	// Core's installation reader currently requires this platform permission.
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") {
		out := Error(403, "NAVIGATION_DENIED")
		out.RequestID = requestID
		return in, "", out
	}
	if n.appCenter != nil {
		view, metadata, result := n.appCenter.read(ctx, token)
		if result.Status != 200 {
			return in, "", result
		}
		if view.Scope != (appcenter.Scope{UserID: in.Scope.UserID, OrganizationID: in.Scope.OrganizationID, StationID: in.Scope.StationID, AuthorizationEpoch: in.Scope.AuthorizationEpoch}) {
			return in, "", navigationError(navigation.ErrInvalid, requestID)
		}
		bindings, err := appCenterNavigationBindings(view, metadata, catalog)
		if err != nil {
			return in, "", navigationError(err, requestID)
		}
		in.Bindings = bindings
	} else {
		bindings, result := n.legacyBindings(ctx, token, requestID, actor, catalog)
		if result.Status != 200 {
			return in, "", result
		}
		in.Bindings = bindings
	}
	prefs, revision, err := n.preferences.Load(ctx, in.Scope)
	if err != nil {
		return in, "", navigationError(err, requestID)
	}
	if !navigation.ValidPreferenceRevision(revision) {
		return in, "", navigationError(navigation.ErrInvalid, requestID)
	}
	in.Preferences = prefs
	return in, revision, Result{Status: 200, RequestID: requestID}
}

// Kept for older embedders; production Web wiring always supplies AppCenter.
func (n *Navigation) legacyBindings(ctx context.Context, token, requestID string, actor dashboard.Actor, catalog navigation.Catalog) ([]navigation.Binding, Result) {
	bindings := []navigation.Binding{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.station.base+"/app-installations", nil)
	if err != nil {
		return nil, navigationError(navigation.ErrUnavailable, requestID)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", requestID)
	resp, err := n.station.client.Do(req)
	if err != nil {
		return nil, navigationError(navigation.ErrUnavailable, requestID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			out := Error(resp.StatusCode, "NAVIGATION_DENIED")
			out.RequestID = requestID
			return nil, out
		}
		return nil, navigationError(navigation.ErrUnavailable, requestID)
	}
	var payload struct {
		RequestID string                  `json:"request_id"`
		StationID string                  `json:"station_id"`
		Items     []dashboardInstallation `json:"items"`
	}
	if strictjson.Decode(resp.Body, 4<<20, &payload) != nil || payload.RequestID != requestID || payload.StationID != actor.StationID || payload.Items == nil || len(payload.Items) > 1000 {
		return nil, navigationError(navigation.ErrInvalid, requestID)
	}
	seenIDs, seenApps := map[string]bool{}, map[string]bool{}
	for _, installed := range payload.Items {
		id, err := uuid.Parse(installed.InstallationID)
		if err != nil || id.Version() != 7 || id.String() != installed.InstallationID || seenIDs[installed.InstallationID] || installed.StationID != actor.StationID || installed.OrganizationID != actor.OrganizationID || !validDashboardInstallation(installed) {
			return nil, navigationError(navigation.ErrInvalid, requestID)
		}
		seenIDs[installed.InstallationID] = true
		if installed.State != "installed" {
			continue
		}
		if seenApps[installed.AppID] {
			return nil, navigationError(navigation.ErrInvalid, requestID)
		}
		seenApps[installed.AppID] = true
		binding, exists := catalog.Releases[installed.AppID+"@"+installed.AppVersion]
		if !exists {
			continue
		}
		binding.InstallationID, binding.OrganizationID, binding.StationID, binding.State = installed.InstallationID, installed.OrganizationID, installed.StationID, installed.State
		bindings = append(bindings, binding)
	}
	return bindings, Result{Status: 200, RequestID: requestID}
}

func navigationResult(in navigationInput, revision string, out Result) Result {
	projection, err := navigation.Resolve(in.Input)
	if err != nil {
		return navigationError(err, out.RequestID)
	}
	out.Data, _ = json.Marshal(NavigationResult{Mode: in.Mode, Projection: projection, PreferenceRevision: revision})
	return out
}

func (n *Navigation) Get(ctx context.Context, token string) Result {
	in, revision, out := n.input(ctx, token)
	if out.Status != 200 {
		return out
	}
	return navigationResult(in, revision, out)
}

func (n *Navigation) Patch(ctx context.Context, token string, patch NavigationPreferencePatch) Result {
	if !dashboardAppID.MatchString(patch.AppID) || !dashboardAppID.MatchString(patch.EntryID) || patch.Order < 0 || patch.Order > 1000000 || !navigation.ValidPreferenceRevision(patch.PreferenceRevision) {
		return Error(400, "INVALID_NAVIGATION_REQUEST")
	}
	in, revision, out := n.input(ctx, token)
	if out.Status != 200 {
		return out
	}
	if in.Mode != "preinstalled" {
		denied := Error(403, "NAVIGATION_DISABLED")
		denied.RequestID = out.RequestID
		return denied
	}
	projection, err := navigation.Resolve(in.Input)
	if err != nil {
		return navigationError(err, out.RequestID)
	}
	found := slices.ContainsFunc(projection.Entries, func(entry navigation.Entry) bool { return entry.AppID == patch.AppID && entry.EntryID == patch.EntryID })
	if !found {
		denied := Error(403, "NAVIGATION_DENIED")
		denied.RequestID = out.RequestID
		return denied
	}
	if revision != patch.PreferenceRevision {
		return navigationError(navigation.ErrConflict, out.RequestID)
	}
	preference := navigation.Preference{AppID: patch.AppID, EntryID: patch.EntryID, Pinned: patch.Pinned, Order: patch.Order}
	index := slices.IndexFunc(in.Preferences.Entries, func(entry navigation.Preference) bool {
		return entry.AppID == patch.AppID && entry.EntryID == patch.EntryID
	})
	if index >= 0 {
		in.Preferences.Entries[index] = preference
	} else {
		if len(in.Preferences.Entries) >= 128 {
			limited := Error(409, "NAVIGATION_PREFERENCE_LIMIT")
			limited.RequestID = out.RequestID
			return limited
		}
		in.Preferences.Entries = append(in.Preferences.Entries, preference)
	}
	revision, err = n.preferences.Save(ctx, in.Scope, in.Preferences, revision)
	if err != nil {
		return navigationError(err, out.RequestID)
	}
	return navigationResult(in, revision, out)
}
