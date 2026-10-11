package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/preinstalled"
)

// AppCenter reads one authenticated Core snapshot. Display/entry providers are
// host-owned; the transport cannot choose endpoints, identities or registries.
type AppCenter struct {
	station          *Station
	now              func() time.Time
	presentationPath string
	declarationsDir  string
	preinstalledPath string
	legacyPath       string
	legacyBackends   func() map[string]string
	dashboards       *Dashboards
}

func NewAppCenter(station *Station, presentationPath string) *AppCenter {
	return &AppCenter{station: station, now: time.Now, presentationPath: presentationPath}
}

// WithDashboards binds the same production resolver used by Dashboard routes.
// Configure only during startup, before serving requests.
func (a *AppCenter) WithDashboards(d *Dashboards, declarationsDir string) *AppCenter {
	a.dashboards, a.declarationsDir = d, declarationsDir
	return a
}

// WithPreinstalled uses the existing host policy only to classify real cards.
// It does not invoke reconciliation or create missing installations.
func (a *AppCenter) WithPreinstalled(path string) *AppCenter {
	a.preinstalledPath = path
	return a
}

func (a *AppCenter) WithLegacy(path string, backends func() map[string]string) *AppCenter {
	a.legacyPath, a.legacyBackends = path, backends
	return a
}

func appCenterError(status int, code, requestID string) Result {
	raw, _ := json.Marshal(struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}{code, requestID})
	return Result{Status: status, Data: raw, RequestID: requestID}
}

func (a *AppCenter) Get(ctx context.Context, token string) Result {
	_, _, out := a.read(ctx, token)
	return out
}

func (a *AppCenter) read(ctx context.Context, token string) (appcenter.View, map[string]appcenter.Metadata, Result) {
	view, metadata, _, out := a.readSnapshot(ctx, token, false)
	return view, metadata, out
}

func (a *AppCenter) readSnapshot(ctx context.Context, token string, manage bool) (appcenter.View, map[string]appcenter.Metadata, appcenter.Facts, Result) {
	id := uuid.Must(uuid.NewV7()).String()
	fail := func(status int, code string) (appcenter.View, map[string]appcenter.Metadata, appcenter.Facts, Result) {
		return appcenter.View{}, nil, appcenter.Facts{}, appCenterError(status, code, id)
	}
	if a == nil || a.station == nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	identity, status := a.station.verifiedIdentity(ctx, token, id)
	if status != 200 {
		if status == 401 {
			return fail(401, "UNAUTHENTICATED")
		}
		if status == 403 {
			return fail(403, "PERMISSION_DENIED")
		}
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") || manage && !slices.Contains(identity.Scopes, "app:manage") {
		return fail(403, "PERMISSION_DENIED")
	}
	actor := dashboardActorFromIdentity(identity)
	scope := appcenter.Scope{UserID: actor.UserID, OrganizationID: actor.OrganizationID, StationID: actor.StationID, AuthorizationEpoch: actor.AuthorizationEpoch}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.station.base+"/appcenter/v1/facts", nil)
	if err != nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Accept", "application/json")
	resp, err := a.station.client.Do(req)
	if err != nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 {
			return fail(401, "UNAUTHENTICATED")
		}
		if resp.StatusCode == 403 {
			return fail(403, "PERMISSION_DENIED")
		}
		if resp.StatusCode == 404 || resp.StatusCode == 405 {
			return fail(502, "APPCENTER_PROTOCOL_UNSUPPORTED")
		}
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	var facts appcenter.Facts
	if appcenter.Decode(resp.Body, "facts", 4<<20, &facts) != nil || facts.RequestID != id || facts.Scope != scope {
		return fail(502, "APPCENTER_INVALID_FACTS")
	}
	groups, metadata, presentation, err := appcenter.LoadPresentation(ctx, a.presentationPath, facts)
	if err != nil {
		if errors.Is(err, appcenter.ErrInvalid) {
			return fail(502, "APPCENTER_INVALID_PRESENTATION")
		}
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	entries := appcenter.Source{Source: "entries", Quality: "unavailable", ReasonCode: appcenter.Text("ENTRY_UNREGISTERED")}
	if a.preinstalledPath != "" {
		config, err := preinstalled.LoadConfig(a.preinstalledPath)
		if err != nil {
			if errors.Is(err, preinstalled.ErrInvalid) {
				return fail(502, "APPCENTER_INVALID_PREINSTALLED")
			}
			return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		if config.OrganizationID == scope.OrganizationID && config.StationID == scope.StationID {
			apps := map[string]bool{}
			for _, release := range config.Apps {
				apps[release.AppID] = true
			}
			for _, fact := range facts.Items {
				meta := metadata[fact.CardKey]
				meta.Preinstalled = apps[fact.AppID]
				metadata[fact.CardKey] = meta
			}
		}
	}
	if a.dashboards != nil {
		registered, err := a.dashboards.source.RegisteredEntries(ctx, a.declarationsDir)
		if err != nil {
			if errors.Is(err, dashboard.ErrInvalid) {
				return fail(502, "APPCENTER_INVALID_ENTRIES")
			}
			return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		entries = appcenter.Source{Source: "entries", Quality: "known", Revision: appcenter.Text(appcenter.Digest(registered))}
		for _, fact := range facts.Items {
			if fact.Installation == nil {
				continue
			}
			e, ok := registered[fact.AppID+"@"+fact.Installation.AppVersion]
			if !ok {
				continue
			}
			meta := metadata[fact.CardKey]
			meta.EntryRevision = e.Revision
			meta.ArtifactDigest = e.ArtifactDigest
			meta.Entry = &appcenter.Entry{EntryID: "dashboard", Transport: "dashboard", Quality: e.Quality,
				Availability: e.Availability, ReasonCode: appcenter.Text(e.ReasonCode), RequiresServiceReady: true}
			metadata[fact.CardKey] = meta
		}
	}
	if a.legacyPath != "" {
		backends := map[string]string{}
		if a.legacyBackends != nil {
			backends = a.legacyBackends()
		}
		bindings, revision, err := appcenter.LoadLegacy(ctx, a.legacyPath, facts, backends)
		if err != nil {
			if errors.Is(err, appcenter.ErrInvalid) {
				return fail(502, "APPCENTER_INVALID_ENTRIES")
			}
			return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		entries = appcenter.Source{Source: "entries", Quality: "known", Revision: appcenter.Text(appcenter.Digest([]any{entries, revision}))}
		for _, fact := range facts.Items {
			binding, ok := bindings[fact.CardKey]
			if !ok {
				continue
			}
			meta := metadata[fact.CardKey]
			if meta.Entry != nil {
				return fail(502, "APPCENTER_INVALID_ENTRIES")
			}
			meta.Legacy = &binding
			meta.EntryRevision = appcenter.Digest([]any{binding, backends[binding.AdapterID]})
			meta.Entry = &appcenter.Entry{EntryID: binding.AdapterID, Transport: "legacy", Quality: "known", Availability: "available", RequiresServiceReady: true}
			if fact.Service.WorkloadUID == nil || *fact.Service.WorkloadUID != binding.WorkloadUID {
				meta.Entry.Availability, meta.Entry.ReasonCode = "unavailable", appcenter.Text("BINDING_CONFLICT")
			} else if backends[binding.AdapterID] == "" || backends[binding.AdapterID] != binding.BackendRevision {
				meta.Entry.Availability, meta.Entry.ReasonCode = "unavailable", appcenter.Text("LEGACY_BACKEND_CHANGED")
			}
			metadata[fact.CardKey] = meta
		}
	}
	now := a.now().UTC()
	view, err := appcenter.Resolve(appcenter.Input{Facts: facts, Scope: scope, Now: now, Groups: groups, Metadata: metadata, Sources: []appcenter.Source{
		presentation,
		entries,
	}})
	if err != nil {
		if errors.Is(err, appcenter.ErrUnavailable) {
			return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
		}
		return fail(502, "APPCENTER_INVALID_FACTS")
	}
	raw, err := json.Marshal(view)
	if err != nil || len(raw) > 4<<20 {
		return fail(503, "APPCENTER_LIMIT_EXCEEDED")
	}
	return view, metadata, facts, Result{Status: 200, Data: raw, RequestID: id}
}
