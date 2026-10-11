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
)

// AppCenter reads one authenticated Core snapshot. Display/entry providers are
// host-owned; the transport cannot choose endpoints, identities or registries.
type AppCenter struct {
	station          *Station
	now              func() time.Time
	presentationPath string
}

func NewAppCenter(station *Station, presentationPath string) *AppCenter {
	return &AppCenter{station: station, now: time.Now, presentationPath: presentationPath}
}

func appCenterError(status int, code, requestID string) Result {
	raw, _ := json.Marshal(struct {
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}{code, requestID})
	return Result{Status: status, Data: raw, RequestID: requestID}
}

func (a *AppCenter) Get(ctx context.Context, token string) Result {
	id := uuid.Must(uuid.NewV7()).String()
	fail := func(status int, code string) Result { return appCenterError(status, code, id) }
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
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") {
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
	// Entry integration is a separate delivery slice. Never guess an entry
	// from an application name or call Apps while listing.
	now := a.now().UTC()
	view, err := appcenter.Resolve(appcenter.Input{Facts: facts, Scope: scope, Now: now, Groups: groups, Metadata: metadata, Sources: []appcenter.Source{
		presentation,
		{Source: "entries", Quality: "unavailable", ReasonCode: appcenter.Text("ENTRY_UNREGISTERED")},
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
	return Result{Status: 200, Data: raw, RequestID: id}
}
