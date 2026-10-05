package application

import (
	"bytes"
	"context"
	"time"

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
	if token == "" {
		return project.Principal{}, 401
	}
	result := s.Call(ctx, token, Request{Path: "me", Method: "GET"})
	if result.Status == 401 || result.Status == 403 {
		return project.Principal{}, 401
	}
	if result.Status != 200 {
		return project.Principal{}, 503
	}
	var identity identityContext
	if strictjson.Decode(bytes.NewReader(result.Data), 64<<10, &identity) != nil {
		return project.Principal{}, 503
	}
	p := project.Principal{OrganizationID: identity.OrganizationID, SubjectID: identity.UserID, RequestID: requestID}
	now := time.Now()
	if !p.Valid() || !project.ValidID(identity.StationID) || !project.ValidID(identity.SessionID) || identity.Username == "" || identity.OrganizationName == "" || len(identity.Roles) == 0 || identity.RevocationVersion < 0 || identity.PolicyVersion < 1 || identity.IssuedAt.IsZero() || identity.IssuedAt.After(now.Add(30*time.Second)) || !identity.ExpiresAt.After(now) || !identity.ExpiresAt.After(identity.IssuedAt) {
		return project.Principal{}, 401
	}
	for _, scope := range identity.Scopes {
		if scope == "identity:read" {
			return p, 200
		}
	}
	return project.Principal{}, 403
}
