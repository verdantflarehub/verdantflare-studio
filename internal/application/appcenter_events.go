package application

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

// AuthorizeEvents verifies current Core authorization without reading App
// state, presentation registries or any user-supplied notification scope.
func (a *AppCenter) AuthorizeEvents(ctx context.Context, token string) (appcenter.Scope, int) {
	if a == nil || a.station == nil {
		return appcenter.Scope{}, 503
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	requestID := uuid.Must(uuid.NewV7()).String()
	identity, status := a.station.sessionIdentity(ctx, token, requestID)
	if status != 200 {
		return appcenter.Scope{}, status
	}
	if identity.RequestID != requestID {
		return appcenter.Scope{}, 503
	}
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") {
		return appcenter.Scope{}, 403
	}
	actor := dashboardActorFromIdentity(identity)
	return appcenter.Scope{UserID: actor.UserID, OrganizationID: actor.OrganizationID, StationID: actor.StationID, AuthorizationEpoch: actor.AuthorizationEpoch}, 200
}
