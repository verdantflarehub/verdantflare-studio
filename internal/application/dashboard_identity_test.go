package application

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// This transport never opens a listener or connects to any service.
type dashboardIdentityTransport func(*http.Request) (*http.Response, error)

func (f dashboardIdentityTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func dashboardIdentityFixture() identityContext {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	return identityContext{StationID: id(), SessionID: id(), UserID: id(), OrganizationID: id(),
		OrganizationName: "Test organization", RequestID: id(), Username: "test-user",
		Roles: []string{"operator", "member"}, Scopes: []string{"identity:read", "app:read"},
		IssuedAt: time.Now().Add(-time.Minute), ExpiresAt: time.Now().Add(time.Hour), RevocationVersion: 1, PolicyVersion: 1}
}

func identityStation(t *testing.T, payload *string, status *int, calls *int) *Station {
	t.Helper()
	return &Station{base: "https://station.invalid", client: &http.Client{Transport: dashboardIdentityTransport(func(r *http.Request) (*http.Response, error) {
		*calls++
		if r.URL.String() != "https://station.invalid/identity/me" || r.Method != "GET" || r.Header.Get("Authorization") != "Bearer test-session" {
			t.Fatal("unexpected identity request")
		}
		return &http.Response{StatusCode: *status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(*payload)), Request: r}, nil
	})}}
}

func identityJSON(identity identityContext) string {
	data, _ := json.Marshal(identity)
	return string(data)
}

func TestDashboardIdentityPreservesScopeAndRevalidatesEveryCall(t *testing.T) {
	identity := dashboardIdentityFixture()
	payload := identityJSON(identity)
	status, calls := 200, 0
	station := identityStation(t, &payload, &status, &calls)
	actor, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID)
	if code != 200 || actor.UserID != identity.UserID || actor.OrganizationID != identity.OrganizationID || actor.StationID != identity.StationID {
		t.Fatalf("scope missing: %d", code)
	}
	if len(actor.AuthorizationEpoch) != 64 || strings.Contains(actor.AuthorizationEpoch, identity.SessionID) {
		t.Fatal("authorization epoch must be opaque")
	}
	principal, code := station.Principal(context.Background(), "test-session", identity.RequestID)
	if code != 200 || principal.SubjectID != actor.UserID || principal.OrganizationID != actor.OrganizationID || principal.RequestID != identity.RequestID {
		t.Fatal("Project principal compatibility changed")
	}
	status = 401
	if _, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID); code != 401 {
		t.Fatal("revoked session accepted")
	}
	if calls != 3 {
		t.Fatalf("identity was cached: %d calls", calls)
	}
	if _, code := station.DashboardActor(context.Background(), "", identity.RequestID); code != 401 || calls != 3 {
		t.Fatal("missing credential reached Core")
	}
}

func TestDashboardIdentityEpochTracksAuthorizationNotDisplayOrOrder(t *testing.T) {
	identity := dashboardIdentityFixture()
	payload := identityJSON(identity)
	status, calls := 200, 0
	station := identityStation(t, &payload, &status, &calls)
	initial, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID)
	if code != 200 {
		t.Fatal(code)
	}
	identity.Roles = []string{"member", "operator"}
	identity.Scopes = []string{"app:read", "identity:read"}
	identity.Username = "renamed"
	identity.RequestID = uuid.Must(uuid.NewV7()).String()
	payload = identityJSON(identity)
	reordered, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID)
	if code != 200 || reordered.AuthorizationEpoch != initial.AuthorizationEpoch {
		t.Fatal("display/order changed authorization epoch")
	}
	for _, change := range []func(*identityContext){
		func(i *identityContext) { i.PolicyVersion++ }, func(i *identityContext) { i.RevocationVersion++ },
		func(i *identityContext) { i.SessionID = uuid.Must(uuid.NewV7()).String() },
		func(i *identityContext) { i.Scopes = append(i.Scopes, "app:manage") },
		func(i *identityContext) { i.Roles = []string{"admin"} },
	} {
		copy := identity
		change(&copy)
		payload = identityJSON(copy)
		actor, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID)
		if code != 200 || actor.AuthorizationEpoch == initial.AuthorizationEpoch {
			t.Fatal("authorization change reused epoch")
		}
	}
}

func TestDashboardIdentityRejectsMalformedExpiredAndUnscopedResponses(t *testing.T) {
	cases := []struct {
		name   string
		change func(*identityContext)
		want   int
	}{
		{"expired", func(i *identityContext) { i.ExpiresAt = time.Now().Add(-time.Second) }, 401},
		{"future", func(i *identityContext) { i.IssuedAt = time.Now().Add(time.Hour) }, 401},
		{"wrong station", func(i *identityContext) { i.StationID = "bad" }, 401},
		{"wrong session", func(i *identityContext) { i.SessionID = "bad" }, 401},
		{"no roles", func(i *identityContext) { i.Roles = []string{} }, 401},
		{"no scope", func(i *identityContext) { i.Scopes = []string{"app:read"} }, 403},
		{"invalid version", func(i *identityContext) { i.PolicyVersion = 0 }, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity := dashboardIdentityFixture()
			tc.change(&identity)
			payload := identityJSON(identity)
			status, calls := 200, 0
			station := identityStation(t, &payload, &status, &calls)
			if _, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID); code != tc.want {
				t.Fatalf("got %d want %d", code, tc.want)
			}
		})
	}
	identity := dashboardIdentityFixture()
	valid := identityJSON(identity)
	for _, payload := range []string{
		strings.Replace(valid, `"policy_version":1`, `"policy_version":1,"policy_version":2`, 1),
		strings.Replace(valid, `"policy_version":1`, `"policy_version":1,"unknown":true`, 1),
		strings.Replace(valid, `"policy_version":1`, `"policy_version":null`, 1),
		strings.Repeat(" ", 64<<10) + valid,
	} {
		status, calls := 200, 0
		station := identityStation(t, &payload, &status, &calls)
		if _, code := station.DashboardActor(context.Background(), "test-session", identity.RequestID); code != 503 {
			t.Fatalf("malformed identity accepted: %d", code)
		}
	}
}
