package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type identityContext struct {
	StationID         string    `json:"station_id"`
	SessionID         string    `json:"session_id"`
	UserID            string    `json:"user_id"`
	OrganizationID    string    `json:"organization_id"`
	OrganizationName  string    `json:"organization_name"`
	RequestID         string    `json:"request_id"`
	Username          string    `json:"username"`
	Roles             []string  `json:"roles"`
	Scopes            []string  `json:"scopes"`
	IssuedAt          time.Time `json:"issued_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	RevocationVersion int64     `json:"revocation_version"`
	PolicyVersion     int64     `json:"policy_version"`
}

// Principal revalidates the actual Core session on every request.
func (s *Station) Principal(ctx context.Context, token, requestID string) (project.Principal, int) {
	identity, status := s.verifiedIdentity(ctx, token, requestID)
	if status != 200 {
		return project.Principal{}, status
	}
	return project.Principal{OrganizationID: identity.OrganizationID, SubjectID: identity.UserID, RequestID: requestID}, 200
}

// DashboardActor retains the Station and authorization lifetime that a Project
// principal alone cannot express. Tokens and Core session IDs stay in Go.
func (s *Station) DashboardActor(ctx context.Context, token, requestID string) (dashboard.Actor, int) {
	identity, status := s.verifiedIdentity(ctx, token, requestID)
	if status != 200 {
		return dashboard.Actor{}, status
	}
	return dashboardActorFromIdentity(identity), 200
}

func dashboardActorFromIdentity(identity identityContext) dashboard.Actor {
	roles, scopes := slices.Clone(identity.Roles), slices.Clone(identity.Scopes)
	slices.Sort(roles)
	slices.Sort(scopes)
	encoded, _ := json.Marshal(struct {
		Session            string
		Revocation, Policy int64
		Roles, Scopes      []string
	}{identity.SessionID, identity.RevocationVersion, identity.PolicyVersion, slices.Compact(roles), slices.Compact(scopes)})
	epoch := sha256.Sum256(encoded)
	return dashboard.Actor{
		UserID: identity.UserID, OrganizationID: identity.OrganizationID,
		StationID: identity.StationID, AuthorizationEpoch: hex.EncodeToString(epoch[:]),
	}
}

func (s *Station) verifiedIdentity(ctx context.Context, token, requestID string) (identityContext, int) {
	if token == "" {
		return identityContext{}, 401
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+"/identity/me", nil)
	if err != nil {
		return identityContext{}, 503
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-Request-ID", requestID)
	result, err := s.client.Do(request)
	if err != nil {
		return identityContext{}, 503
	}
	defer result.Body.Close()
	if result.StatusCode == 401 || result.StatusCode == 403 {
		return identityContext{}, 401
	}
	if result.StatusCode != 200 {
		return identityContext{}, 503
	}
	var identity identityContext
	// Decode original bytes. The general UI proxy rewrites JSON maps, which would
	// erase duplicate identity fields before strict decoding could reject them.
	if strictjson.Decode(result.Body, 64<<10, &identity) != nil {
		return identityContext{}, 503
	}
	p := project.Principal{OrganizationID: identity.OrganizationID, SubjectID: identity.UserID, RequestID: requestID}
	now := time.Now()
	if !p.Valid() || !project.ValidID(identity.StationID) || !project.ValidID(identity.SessionID) || identity.Username == "" || identity.OrganizationName == "" || len(identity.Roles) == 0 || identity.RevocationVersion < 0 || identity.PolicyVersion < 1 || identity.IssuedAt.IsZero() || identity.IssuedAt.After(now.Add(30*time.Second)) || !identity.ExpiresAt.After(now) || !identity.ExpiresAt.After(identity.IssuedAt) {
		return identityContext{}, 401
	}
	for _, scope := range identity.Scopes {
		if scope == "identity:read" {
			return identity, 200
		}
	}
	return identityContext{}, 403
}
