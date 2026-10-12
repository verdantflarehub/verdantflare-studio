package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGenerationIdentityRequiresFreshSameSubjectAndRevokesTemporarySession(t *testing.T) {
	for _, scenario := range []string{"success", "wrong_password", "switched_subject", "reused_session", "non_admin"} {
		t.Run(scenario, func(t *testing.T) {
			id := func() string { return uuid.Must(uuid.NewV7()).String() }
			identity := identityContext{StationID: id(), SessionID: id(), UserID: id(), OrganizationID: id(), OrganizationName: "test", RequestID: id(), Username: "fixture", Roles: []string{"admin"}, Scopes: []string{"identity:read"}, IssuedAt: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour), PolicyVersion: 1}
			fresh := identity
			fresh.SessionID = id()
			fresh.IssuedAt = time.Now()
			if scenario == "switched_subject" {
				fresh.UserID = id()
			}
			if scenario == "reused_session" {
				fresh.SessionID = identity.SessionID
			}
			if scenario == "non_admin" {
				identity.Roles = []string{"reader"}
			}
			logins, logouts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/identity/me":
					v := identity
					if r.Header.Get("Authorization") == "Bearer temporary" {
						v = fresh
					}
					_ = json.NewEncoder(w).Encode(v)
				case "/identity/login":
					logins++
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["username"] != identity.Username || body["organization_id"] != identity.OrganizationID || body["password"] != "typed-confirmation" || r.Header.Get("Authorization") != "" {
						t.Error("invalid step-up scope")
					}
					if scenario == "wrong_password" {
						w.WriteHeader(401)
						_, _ = w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
						return
					}
					w.WriteHeader(201)
					_, _ = w.Write([]byte(`{"access_token":"temporary","refresh_token":"never-return"}`))
				case "/identity/logout":
					logouts++
					if r.Header.Get("Authorization") != "Bearer temporary" {
						t.Error("revoked original session")
					}
					w.WriteHeader(204)
				default:
					t.Error("unexpected identity operation")
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			station, _ := NewStation(server.URL)
			p, status := station.ConfirmGenerationIdentity(context.Background(), "original", identity.RequestID, "typed-confirmation")
			if scenario == "success" {
				if status != 200 || p.SubjectID != identity.UserID {
					t.Fatal(status, p)
				}
			} else if status == 200 {
				t.Fatal("invalid confirmation accepted")
			}
			if scenario == "non_admin" {
				if logins != 0 {
					t.Fatal("unauthorized password exchange")
				}
			} else if logins != 1 {
				t.Fatal("unexpected reauthentication count", logins)
			}
			wantLogout := 1
			if scenario == "wrong_password" || scenario == "non_admin" {
				wantLogout = 0
			}
			if logouts != wantLogout {
				t.Fatal("temporary session not revoked", logouts)
			}
		})
	}
}
