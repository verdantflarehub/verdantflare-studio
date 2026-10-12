package application

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// ConfirmGenerationIdentity is a fresh, request-specific authentication, not a
// reinterpretation of an existing MCP bearer as evidence of human approval.
// The temporary login token is never returned and is immediately revoked.
func (s *Station) ConfirmGenerationIdentity(ctx context.Context, token, requestID, password string) (project.Principal, int) {
	if len(password) < 1 || len(password) > 1024 {
		return project.Principal{}, 400
	}
	current, status := s.verifiedIdentity(ctx, token, requestID)
	if status != 200 {
		return project.Principal{}, status
	}
	if !slices.Contains(current.Roles, "admin") {
		return project.Principal{}, 403
	}
	body, _ := json.Marshal(struct {
		Username       string `json:"username"`
		Password       string `json:"password"`
		OrganizationID string `json:"organization_id"`
	}{current.Username, password, current.OrganizationID})
	login := s.Call(ctx, "", Request{Path: "login", Method: "POST", Body: body})
	if login.Status != 201 || login.Token == "" {
		if login.Status >= 500 {
			return project.Principal{}, 503
		}
		return project.Principal{}, 403
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.Call(cleanup, login.Token, Request{Path: "logout", Method: "POST"})
	}()
	fresh, status := s.verifiedIdentity(ctx, login.Token, requestID)
	if status != 200 {
		return project.Principal{}, status
	}
	if fresh.UserID != current.UserID || fresh.OrganizationID != current.OrganizationID || fresh.StationID != current.StationID || !slices.Contains(fresh.Roles, "admin") || fresh.SessionID == current.SessionID || fresh.IssuedAt.Before(time.Now().Add(-time.Minute)) {
		return project.Principal{}, 403
	}
	return project.Principal{OrganizationID: fresh.OrganizationID, SubjectID: fresh.UserID, RequestID: requestID}, 200
}
