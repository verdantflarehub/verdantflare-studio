package application

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAppCenterEventsVerifiedIdentityOnly(t *testing.T) {
	for _, scenario := range []string{"authorized", "role", "scope", "expired", "unavailable", "wrong-request"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now().UTC()
			newID := func() string { return uuid.Must(uuid.NewV7()).String() }
			id := identityContext{UserID: newID(), OrganizationID: newID(), StationID: newID(), SessionID: newID(), Username: "test-user", OrganizationName: "test-org", Roles: []string{"admin"}, Scopes: []string{"identity:read", "app:read"}, IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), PolicyVersion: 1}
			switch scenario {
			case "role":
				id.Roles = []string{"member"}
			case "scope":
				id.Scopes = []string{"identity:read"}
			case "expired":
				id.ExpiresAt = now.Add(-time.Second)
			}
			calls := 0
			station := &Station{base: "https://core.invalid", client: &http.Client{Transport: appCenterTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Path != "/identity/me" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-session" {
					t.Fatal("notification auth read facts or used wrong credentials")
				}
				id.RequestID = r.Header.Get("X-Request-ID")
				if scenario == "wrong-request" {
					id.RequestID = "other-request"
				}
				status := 200
				if scenario == "unavailable" {
					status = 503
				}
				raw, _ := json.Marshal(id)
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})}}
			scope, status := NewAppCenter(station, "").AuthorizeEvents(t.Context(), "test-session")
			if calls != 1 {
				t.Fatal("unexpected auth calls")
			}
			if scenario == "authorized" {
				if status != 200 || scope.UserID != id.UserID || scope.OrganizationID != id.OrganizationID || scope.StationID != id.StationID || scope.AuthorizationEpoch != dashboardActorFromIdentity(id).AuthorizationEpoch {
					t.Fatal("wrong verified scope")
				}
			} else if status == 200 || scope.UserID != "" || scope.OrganizationID != "" || scope.StationID != "" {
				t.Fatal("invalid authorization produced subscription scope")
			}
		})
	}
}
