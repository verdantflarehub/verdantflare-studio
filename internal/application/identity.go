package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"sync"
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

// identityCacheTTL is the longest a permission change or revoked session can
// remain unobserved by Studio reads; Core still checks every command itself.
const identityCacheTTL = 15 * time.Second

type cachedIdentity struct {
	identity identityContext
	expires  time.Time
}

type identityFlight struct {
	done     chan struct{}
	identity identityContext
	status   int
}

// identityCache keeps verified identities per session token. Concurrent reads
// of one token share a single Core request; only 200 results are retained.
type identityCache struct {
	mu      sync.Mutex
	entries map[string]cachedIdentity
	flights map[string]*identityFlight
}

func identityKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ForgetIdentity drops a session identity after logout or rejection.
func (s *Station) ForgetIdentity(token string) {
	if s == nil || token == "" {
		return
	}
	key := identityKey(token)
	s.identities.mu.Lock()
	delete(s.identities.entries, key)
	s.identities.mu.Unlock()
}

// sessionIdentity serves the refresh path (views, navigation, Open and state
// hints) from a short per-session cache. Proxies and long-lived workspaces
// keep calling verifiedIdentity so revocation stays immediate there.
func (s *Station) sessionIdentity(ctx context.Context, token, requestID string) (identityContext, int) {
	if token == "" {
		return identityContext{}, 401
	}
	if s.identityTTL <= 0 {
		return s.verifiedIdentity(ctx, token, requestID)
	}
	key, now := identityKey(token), time.Now()
	c := &s.identities
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		if now.Before(entry.expires) {
			c.mu.Unlock()
			identity := entry.identity
			identity.RequestID = requestID
			identity.Roles, identity.Scopes = slices.Clone(identity.Roles), slices.Clone(identity.Scopes)
			return identity, 200
		}
		delete(c.entries, key)
	}
	flight, waiting := c.flights[key]
	if !waiting {
		if c.flights == nil {
			c.flights = map[string]*identityFlight{}
		}
		flight = &identityFlight{done: make(chan struct{})}
		c.flights[key] = flight
	}
	c.mu.Unlock()
	if waiting {
		select {
		case <-flight.done:
		case <-ctx.Done():
			return identityContext{}, 503
		}
		if flight.status != 200 {
			return identityContext{}, flight.status
		}
		identity := flight.identity
		identity.RequestID = requestID
		identity.Roles, identity.Scopes = slices.Clone(identity.Roles), slices.Clone(identity.Scopes)
		return identity, 200
	}
	identity, status := s.verifiedIdentity(ctx, token, requestID)
	c.mu.Lock()
	delete(c.flights, key)
	if status == 200 {
		expires := now.Add(s.identityTTL)
		if identity.ExpiresAt.Before(expires) {
			expires = identity.ExpiresAt
		}
		if c.entries == nil {
			c.entries = map[string]cachedIdentity{}
		}
		if len(c.entries) >= 4096 {
			for k, e := range c.entries {
				if !now.Before(e.expires) {
					delete(c.entries, k)
				}
			}
		}
		if len(c.entries) < 4096 {
			stored := identity
			stored.Roles, stored.Scopes = slices.Clone(identity.Roles), slices.Clone(identity.Scopes)
			c.entries[key] = cachedIdentity{identity: stored, expires: expires}
		}
	} else if status == 401 || status == 403 {
		delete(c.entries, key)
	}
	flight.identity, flight.status = identity, status
	close(flight.done)
	c.mu.Unlock()
	return identity, status
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
